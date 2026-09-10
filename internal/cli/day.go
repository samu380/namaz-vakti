package cli

import (
	"fmt"
	"time"

	"github.com/samu380/namaz-vakti/internal/cache"
	"github.com/samu380/namaz-vakti/internal/config"
	"github.com/samu380/namaz-vakti/pkg/vakit"
)

// dayMode unterscheidet die drei Varianten der Tagesansicht.
type dayMode int

const (
	dayToday dayMode = iota
	dayTomorrow
	dayExplicit // Datum kommt als Argument
)

// dateLayout ist das Eingabeformat von `namaz tarih`.
const inputDateLayout = "2006-01-02"

// resolved bündelt Standort und Zeitzone.
type resolved struct {
	Key string // Config-Schlüssel, z. B. "ev"
	Loc config.Location
	TZ  *time.Location
}

// resolveLocation ermittelt den zu verwendenden Standort aus -k bzw. der
// Vorgabe in der Config und lädt dessen Zeitzone.
func (e *env) resolveLocation() (resolved, error) {
	key, loc, err := e.cfg.Resolve(e.g.Location)
	if err != nil {
		return resolved{}, err
	}
	tz, err := loc.LoadTZ()
	if err != nil {
		return resolved{}, fmt.Errorf("konum %q: %w", key, err)
	}
	return resolved{Key: key, Loc: loc, TZ: tz}, nil
}

// loadCalendar holt den Kalender und wendet die konfigurierten
// Minutenkorrekturen an.
//
// need ist der Tag, der enthalten sein muss; fehlt er im Cache, wird
// nachgeladen (sofern nicht --cevrimdisi gesetzt ist).
func (e *env) loadCalendar(r resolved, need time.Time) (vakit.Calendar, cache.Meta, error) {
	ctx, cancel := e.ctx()
	defer cancel()

	cal, meta, err := e.cache.Calendar(ctx, r.Loc.DistrictID, r.TZ, need)
	if err != nil {
		return vakit.Calendar{}, cache.Meta{}, err
	}
	return cal.WithOffsets(e.cfg.OffsetDurations()), meta, nil
}

// cmdDay bedient `namaz`, `namaz bugun`, `namaz yarin` und `namaz tarih`.
func cmdDay(e *env, args []string, mode dayMode) error {
	name := map[dayMode]string{dayToday: "bugun", dayTomorrow: "yarin", dayExplicit: "tarih"}[mode]
	fs := e.subFlags(name)
	noKerahat := fs.Bool("kerahat-yok", false, "kerahat satırını gizle")
	rest, err := e.parse(fs, args)
	if err != nil {
		return err
	}
	if e.g.Help {
		fs.Usage()
		return nil
	}

	r, err := e.resolveLocation()
	if err != nil {
		return err
	}

	// Bezugszeitpunkt in der Zeitzone des Standorts. Wichtig, wenn man von
	// Deutschland aus die Zeiten für Istanbul abfragt: dort kann bereits der
	// nächste Tag angebrochen sein.
	now := e.now.In(r.TZ)

	var target time.Time
	switch mode {
	case dayToday:
		target = now
	case dayTomorrow:
		target = now.AddDate(0, 0, 1)
	case dayExplicit:
		if len(rest) == 0 {
			return fmt.Errorf("tarih gerekli, örn: namaz tarih %s", now.Format(inputDateLayout))
		}
		t, err := time.ParseInLocation(inputDateLayout, rest[0], r.TZ)
		if err != nil {
			return fmt.Errorf("tarih çözümlenemedi %q (beklenen biçim: YYYY-AA-GG)", rest[0])
		}
		target = t
	}

	cal, meta, err := e.loadCalendar(r, target)
	if err != nil {
		return err
	}

	day, ok := cal.FindDate(target)
	if !ok {
		first, last := cal.Range()
		return fmt.Errorf("%s takvimde yok (kapsam: %s – %s)",
			target.Format(inputDateLayout),
			first.Format(inputDateLayout), last.Format(inputDateLayout))
	}

	source := jsonSource{
		Provider:  e.cfg.Provider,
		FromCache: meta.FromCache,
		FetchedAt: iso(meta.FetchedAt),
	}
	dur := e.cfg.Kerahat.Durations()

	// Nur für heute gibt es einen sinnvollen Countdown.
	if mode == dayToday {
		st, err := cal.StatusAt(now, dur)
		if err != nil {
			return err
		}
		if *noKerahat {
			st.Kerahat = nil
		}
		if e.g.JSON {
			return writeJSONOut(e.stdout, buildStatusJSON(r, st, dur, source))
		}
		e.renderStatusDay(e.stdout, r.Key, r.Loc.Name, st)
		return nil
	}

	if e.g.JSON {
		return writeJSONOut(e.stdout, buildDayJSON(r, day, dur, source))
	}
	e.renderPlainDay(e.stdout, r.Key, r.Loc.Name, day)
	return nil
}

// buildStatusJSON baut die JSON-Ausgabe für heute, inklusive Countdown.
func buildStatusJSON(r resolved, st vakit.Status, dur vakit.KerahatDurations, src jsonSource) jsonDay {
	out := baseDayJSON(r, st.Today, dur, src)
	out.Kerahat = buildKerahats(st.Today, dur, st.Now, true)

	if st.Current != nil {
		rem, _ := st.Remaining()
		out.Current = &jsonCurrent{
			Prayer:       st.Current.Prayer,
			Opens:        st.Current.Opens.Key(),
			End:          hhmm(st.Current.To),
			EndISO:       iso(st.Current.To),
			RemainingSec: int(rem.Seconds()),
		}
	}
	if st.Next != nil {
		until, _ := st.UntilNext()
		out.Next = &jsonNext{
			Key:          st.Next.Kind.Key(),
			Prayer:       st.Next.Prayer,
			Clock:        hhmm(st.Next.At),
			ISO:          iso(st.Next.At),
			RemainingSec: int(until.Seconds()),
		}
	}
	return out
}

// buildDayJSON baut die JSON-Ausgabe für einen anderen Tag als heute.
func buildDayJSON(r resolved, d vakit.Day, dur vakit.KerahatDurations, src jsonSource) jsonDay {
	out := baseDayJSON(r, d, dur, src)
	out.Kerahat = buildKerahats(d, dur, time.Time{}, false)
	return out
}

// baseDayJSON füllt die Felder, die beide Varianten gemeinsam haben.
func baseDayJSON(r resolved, d vakit.Day, dur vakit.KerahatDurations, src jsonSource) jsonDay {
	return jsonDay{
		Location: jsonLocation{
			Key:        r.Key,
			Name:       r.Loc.Name,
			DistrictID: r.Loc.DistrictID,
			TZ:         r.Loc.TZ,
		},
		Date: jsonDate{
			Gregorian: d.Date.Format(inputDateLayout),
			Weekday:   d.Weekday(),
			Hijri:     d.HijriLong,
		},
		Times:   buildTimes(d),
		Sunrise: hhmm(d.Sunrise),
		Sunset:  hhmm(d.Sunset),
		Qibla:   hhmm(d.QiblaTime),
		Source:  src,
	}
}
