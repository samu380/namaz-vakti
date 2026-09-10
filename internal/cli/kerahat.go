package cli

import (
	"fmt"
	"time"
)

// cmdKerahat bedient `namaz kerahat` — die drei Fenster eines Tages am Stück.
//
// Die Tagesansicht meldet nur die *laufende* Kerahat, damit sie ruhig bleibt.
// Dieses Kommando ist für den Blick nach vorn: "wann ist heute İstiva, ich
// will davor noch nafile beten".
func cmdKerahat(e *env, args []string) error {
	fs := e.subFlags("kerahat")
	rest, err := e.parse(fs, args)
	if err != nil {
		return err
	}
	if e.g.Help {
		fs.Usage()
		fmt.Fprint(e.stderr, "\nİsteğe bağlı: namaz kerahat 2026-12-24\n")
		return nil
	}

	r, err := e.resolveLocation()
	if err != nil {
		return err
	}
	now := e.now.In(r.TZ)

	// Optionales Datum als freies Argument.
	target := now
	isToday := true
	if len(rest) > 0 {
		t, err := time.ParseInLocation(inputDateLayout, rest[0], r.TZ)
		if err != nil {
			return fmt.Errorf("tarih çözümlenemedi %q (beklenen biçim: YYYY-AA-GG)", rest[0])
		}
		target, isToday = t, false
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

	dur := e.cfg.Kerahat.Durations()

	if e.g.JSON {
		out := baseDayJSON(r, day, dur, jsonSource{
			Provider:  e.cfg.Provider,
			FromCache: meta.FromCache,
			FetchedAt: iso(meta.FetchedAt),
		})
		out.Times = nil
		out.Kerahat = buildKerahats(day, dur, now, isToday)
		return writeJSONOut(e.stdout, out)
	}

	fmt.Fprintln(e.stdout)
	e.renderHeader(e.stdout, r.Key, r.Loc.Name, day)
	fmt.Fprintln(e.stdout)

	for _, k := range day.Kerahats(dur) {
		line := fmt.Sprintf("    %s%s – %s   %s",
			padRight(k.Kind.Label(), 10),
			hhmm(k.Start), hhmm(k.End),
			e.style.dim(k.Kind.Note()))

		if isToday && k.Contains(now) {
			line += "  " + e.style.warn(fmt.Sprintf("▸ şu an, %s kaldı",
				FormatDuration(k.End.Sub(now), false)))
		}
		fmt.Fprintln(e.stdout, line)
	}
	fmt.Fprintln(e.stdout)
	return nil
}
