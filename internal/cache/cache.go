// Package cache legt die API-Antworten auf der Platte ab.
//
// Der ezanvakti-Dienst liefert rund 32 Tage am Stück. Ein einziger Abruf
// deckt damit gut vier Wochen ab, weshalb das Werkzeug im Normalbetrieb
// vollständig offline und ohne Netzwerklatenz arbeitet. Das ist nicht nur
// bequem, sondern Voraussetzung dafür, dass später eine Menüleiste `namaz
// sonraki` im Minutentakt aufrufen kann, ohne den Dienst zu belasten.
//
// Gespeichert werden Uhrzeiten bewusst als "HH:MM"-Wandzeit und nicht als
// absoluter Zeitstempel. Beim Laden wird die Zeitzone erneut angewendet,
// sodass ein über die Sommerzeitumstellung hinweg gecachter Kalender korrekt
// bleibt.
package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/samu380/namaz-vakti/internal/provider"
	"github.com/samu380/namaz-vakti/pkg/vakit"
)

// ErrOffline meldet, dass Daten fehlen und wegen --çevrimdışı nicht geladen
// werden dürfen.
var ErrOffline = errors.New("çevrimdışı mod: önbellekte veri yok")

// placeTTL bestimmt, wie lange die Standortlisten (Länder, Bundesländer,
// Kreise) als gültig gelten. Diyanets Kreisliste ändert sich praktisch nie.
const placeTTL = 30 * 24 * time.Hour

// Meta beschreibt die Herkunft eines gelieferten Kalenders.
type Meta struct {
	// FromCache ist true, wenn kein Netzwerkzugriff nötig war.
	FromCache bool
	// FetchedAt ist der Zeitpunkt des letzten erfolgreichen Abrufs.
	FetchedAt time.Time
	// First und Last sind der abgedeckte Zeitraum.
	First, Last time.Time
}

// Cache umschließt einen Provider und bedient Anfragen wenn möglich von der
// Platte.
type Cache struct {
	dir string
	p   provider.Provider

	// Offline verbietet jeden Netzwerkzugriff.
	Offline bool
	// Refresh erzwingt einen Abruf, auch wenn der Cache gültig wäre.
	Refresh bool
}

// New erzeugt einen Cache im Verzeichnis dir.
func New(dir string, p provider.Provider) *Cache {
	return &Cache{dir: dir, p: p}
}

// Dir gibt das Cache-Verzeichnis zurück.
func (c *Cache) Dir() string { return c.dir }

// --- Serialisierungsformat --------------------------------------------------

// storedCalendar ist das Ablageformat. Es ist bewusst vom Domänenmodell
// entkoppelt, damit eine spätere Änderung an vakit.Day nicht stillschweigend
// alte Cache-Dateien fehlinterpretiert (siehe Version).
type storedCalendar struct {
	Version    int         `json:"surum"`
	Provider   string      `json:"saglayici"`
	DistrictID string      `json:"ilce_id"`
	TZ         string      `json:"saat_dilimi"`
	FetchedAt  time.Time   `json:"alindi"`
	Days       []storedDay `json:"gunler"`
}

type storedDay struct {
	Date       string    `json:"tarih"` // "2006-01-02"
	Times      [6]string `json:"vakitler"`
	Sunrise    string    `json:"gunes_dogus"`
	Sunset     string    `json:"gunes_batis"`
	Qibla      string    `json:"kible_saati"`
	HijriShort string    `json:"hicri_kisa"`
	HijriLong  string    `json:"hicri_uzun"`
}

// storeVersion wird bei inkompatiblen Formatänderungen erhöht; ältere
// Dateien werden dann verworfen und neu geladen.
const storeVersion = 1

const (
	dateLayout  = "2006-01-02"
	clockLayout = "15:04"
)

func encodeCalendar(p, districtID string, cal vakit.Calendar, fetched time.Time) storedCalendar {
	hhmm := func(t time.Time) string {
		if t.IsZero() {
			return ""
		}
		return t.Format(clockLayout)
	}

	out := storedCalendar{
		Version:    storeVersion,
		Provider:   p,
		DistrictID: districtID,
		TZ:         cal.Location.String(),
		FetchedAt:  fetched,
		Days:       make([]storedDay, len(cal.Days)),
	}
	for i, d := range cal.Days {
		sd := storedDay{
			Date:       d.Date.Format(dateLayout),
			Sunrise:    hhmm(d.Sunrise),
			Sunset:     hhmm(d.Sunset),
			Qibla:      hhmm(d.QiblaTime),
			HijriShort: d.HijriShort,
			HijriLong:  d.HijriLong,
		}
		for k := range d.Times {
			sd.Times[k] = d.Times[k].Format(clockLayout)
		}
		out.Days[i] = sd
	}
	return out
}

// decode baut den Kalender wieder auf und wendet dabei loc an. Dadurch
// stimmen die Zeiten auch dann, wenn seit dem Abruf die Sommerzeit gewechselt
// hat.
func (s storedCalendar) decode(loc *time.Location) (vakit.Calendar, error) {
	cal := vakit.Calendar{Location: loc, Days: make([]vakit.Day, 0, len(s.Days))}

	for _, sd := range s.Days {
		date, err := time.ParseInLocation(dateLayout, sd.Date, loc)
		if err != nil {
			return vakit.Calendar{}, fmt.Errorf("önbellek tarihi bozuk %q: %w", sd.Date, err)
		}

		at := func(hhmm string) time.Time {
			if hhmm == "" {
				return time.Time{}
			}
			t, err := time.Parse(clockLayout, hhmm)
			if err != nil {
				return time.Time{}
			}
			return time.Date(date.Year(), date.Month(), date.Day(), t.Hour(), t.Minute(), 0, 0, loc)
		}

		d := vakit.Day{
			Date:       date,
			Sunrise:    at(sd.Sunrise),
			Sunset:     at(sd.Sunset),
			QiblaTime:  at(sd.Qibla),
			HijriShort: sd.HijriShort,
			HijriLong:  sd.HijriLong,
		}
		for k := range d.Times {
			d.Times[k] = at(sd.Times[k])
		}
		cal.Days = append(cal.Days, d)
	}
	return cal, nil
}

// --- Kalenderzugriff --------------------------------------------------------

// calendarPath ist der Ablageort für einen Kreis.
func (c *Cache) calendarPath(districtID string) string {
	return filepath.Join(c.dir, fmt.Sprintf("vakitler-%s-%s.json", c.p.Name(), districtID))
}

// Calendar liefert den Kalender für einen Kreis.
//
// need ist der Tag, der mindestens enthalten sein muss. Geprüft wird
// zusätzlich der Folgetag, weil das Yatsı-Fenster bis zum İmsak des nächsten
// Tages reicht. Fehlt einer der beiden, wird neu geladen.
func (c *Cache) Calendar(ctx context.Context, districtID string, loc *time.Location, need time.Time) (vakit.Calendar, Meta, error) {
	path := c.calendarPath(districtID)

	if !c.Refresh {
		if stored, err := readJSON[storedCalendar](path); err == nil &&
			stored.Version == storeVersion && stored.Provider == c.p.Name() {

			if cal, err := stored.decode(loc); err == nil && covers(cal, need) {
				first, last := cal.Range()
				return cal, Meta{
					FromCache: true,
					FetchedAt: stored.FetchedAt,
					First:     first,
					Last:      last,
				}, nil
			}
		}
	}

	if c.Offline {
		return vakit.Calendar{}, Meta{}, fmt.Errorf("%w (%s)", ErrOffline, path)
	}

	cal, err := c.p.Calendar(ctx, districtID, loc)
	if err != nil {
		// Beim Netzwerkfehler auf einen vorhandenen, aber unvollständigen
		// Cache zurückfallen ist keine gute Idee: der Nutzer bekäme still
		// falsche Zeiten. Lieber ehrlich scheitern.
		return vakit.Calendar{}, Meta{}, err
	}

	now := time.Now()
	if err := writeJSON(path, encodeCalendar(c.p.Name(), districtID, cal, now)); err != nil {
		// Ein fehlgeschlagener Cache-Schreibvorgang darf die Ausgabe nicht
		// verhindern — nur melden.
		fmt.Fprintf(os.Stderr, "uyarı: önbellek yazılamadı: %v\n", err)
	}

	first, last := cal.Range()
	return cal, Meta{FromCache: false, FetchedAt: now, First: first, Last: last}, nil
}

// covers prüft, ob need und der Folgetag im Kalender liegen.
func covers(cal vakit.Calendar, need time.Time) bool {
	return cal.Covers(need) && cal.Covers(need.AddDate(0, 0, 1))
}

// CalendarInfo liest die Cache-Metadaten, ohne etwas zu laden. Wird von
// `namaz ayar` für die Cache-Zeile verwendet.
func (c *Cache) CalendarInfo(districtID string) (Meta, bool) {
	stored, err := readJSON[storedCalendar](c.calendarPath(districtID))
	if err != nil || len(stored.Days) == 0 {
		return Meta{}, false
	}
	utc := time.UTC
	first, _ := time.ParseInLocation(dateLayout, stored.Days[0].Date, utc)
	last, _ := time.ParseInLocation(dateLayout, stored.Days[len(stored.Days)-1].Date, utc)
	return Meta{FromCache: true, FetchedAt: stored.FetchedAt, First: first, Last: last}, true
}

// --- Standortlisten ---------------------------------------------------------

type storedPlaces struct {
	FetchedAt time.Time        `json:"alindi"`
	Places    []provider.Place `json:"kayitlar"`
}

func (c *Cache) placesPath(kind, id string) string {
	if id == "" {
		return filepath.Join(c.dir, fmt.Sprintf("%s-%s.json", kind, c.p.Name()))
	}
	return filepath.Join(c.dir, fmt.Sprintf("%s-%s-%s.json", kind, c.p.Name(), id))
}

// places holt eine Standortliste aus dem Cache oder über fetch.
func (c *Cache) places(ctx context.Context, kind, id string, fetch func(context.Context) ([]provider.Place, error)) ([]provider.Place, error) {
	path := c.placesPath(kind, id)

	if !c.Refresh {
		if sp, err := readJSON[storedPlaces](path); err == nil &&
			time.Since(sp.FetchedAt) < placeTTL && len(sp.Places) > 0 {
			return sp.Places, nil
		}
	}
	if c.Offline {
		return nil, fmt.Errorf("%w (%s)", ErrOffline, path)
	}

	list, err := fetch(ctx)
	if err != nil {
		return nil, err
	}
	if err := writeJSON(path, storedPlaces{FetchedAt: time.Now(), Places: list}); err != nil {
		fmt.Fprintf(os.Stderr, "uyarı: önbellek yazılamadı: %v\n", err)
	}
	return list, nil
}

func (c *Cache) Countries(ctx context.Context) ([]provider.Place, error) {
	return c.places(ctx, "ulkeler", "", c.p.Countries)
}

func (c *Cache) States(ctx context.Context, countryID string) ([]provider.Place, error) {
	return c.places(ctx, "sehirler", countryID, func(ctx context.Context) ([]provider.Place, error) {
		return c.p.States(ctx, countryID)
	})
}

func (c *Cache) Districts(ctx context.Context, stateID string) ([]provider.Place, error) {
	return c.places(ctx, "ilceler", stateID, func(ctx context.Context) ([]provider.Place, error) {
		return c.p.Districts(ctx, stateID)
	})
}

// LookupDistrict sucht einen Kreis anhand seiner ID in den bereits
// zwischengespeicherten Kreislisten.
//
// Damit kann `namaz konum ekle --ilce 10532` den Anzeigenamen "WEINHEIM"
// selbst ermitteln, wenn zuvor `namaz konum ara` lief — ohne dafür erneut
// ans Netz zu gehen. Findet sich nichts, ist das kein Fehler: der Aufrufer
// fällt dann auf die ID zurück.
func (c *Cache) LookupDistrict(id string) (provider.Place, bool) {
	pattern := filepath.Join(c.dir, fmt.Sprintf("ilceler-%s-*.json", c.p.Name()))
	files, err := filepath.Glob(pattern)
	if err != nil {
		return provider.Place{}, false
	}
	for _, f := range files {
		sp, err := readJSON[storedPlaces](f)
		if err != nil {
			continue
		}
		for _, p := range sp.Places {
			if p.ID == id {
				return p, true
			}
		}
	}
	return provider.Place{}, false
}

// DistrictChain ermittelt aus dem Cache, zu welchem Bundesland und Land ein
// Kreis gehört.
//
// Damit kann `namaz konum ekle --ilce 10532` ohne --sd die passende Zeitzone
// aus dem Land ableiten. Die Alternative — die Systemzeitzone — ist zwar
// regeltechnisch richtig, liefert auf macOS aber den zusammengelegten
// tzdata-Namen (für Deutschland "Europe/Oslo"), was in einer
// Konfigurationsdatei für Weinheim nur verwirrt.
//
// Voraussetzung ist, dass vorher `namaz konum ara` lief. Fehlt etwas im
// Cache, ist ok false und der Aufrufer weicht auf die Systemzeitzone aus.
func (c *Cache) DistrictChain(districtID string) (country, state provider.Place, ok bool) {
	// Schritt 1: Kreis in einer der Kreislisten finden. Der Dateiname trägt
	// die Bundesland-ID.
	stateID := ""
	pattern := filepath.Join(c.dir, fmt.Sprintf("ilceler-%s-*.json", c.p.Name()))
	files, _ := filepath.Glob(pattern)
	for _, f := range files {
		sp, err := readJSON[storedPlaces](f)
		if err != nil {
			continue
		}
		for _, p := range sp.Places {
			if p.ID == districtID {
				stateID = idFromFilename(f)
				break
			}
		}
		if stateID != "" {
			break
		}
	}
	if stateID == "" {
		return provider.Place{}, provider.Place{}, false
	}

	// Schritt 2: Bundesland in einer der Bundeslandlisten finden. Deren
	// Dateiname trägt die Land-ID.
	countryID := ""
	pattern = filepath.Join(c.dir, fmt.Sprintf("sehirler-%s-*.json", c.p.Name()))
	files, _ = filepath.Glob(pattern)
	for _, f := range files {
		sp, err := readJSON[storedPlaces](f)
		if err != nil {
			continue
		}
		for _, p := range sp.Places {
			if p.ID == stateID {
				state, countryID = p, idFromFilename(f)
				break
			}
		}
		if countryID != "" {
			break
		}
	}
	if countryID == "" {
		return provider.Place{}, provider.Place{}, false
	}

	// Schritt 3: Land in der Länderliste nachschlagen.
	sp, err := readJSON[storedPlaces](c.placesPath("ulkeler", ""))
	if err != nil {
		return provider.Place{}, state, false
	}
	for _, p := range sp.Places {
		if p.ID == countryID {
			return p, state, true
		}
	}
	return provider.Place{}, state, false
}

// idFromFilename zieht die ID aus einem Cache-Dateinamen der Form
// "<art>-<provider>-<id>.json".
func idFromFilename(path string) string {
	base := strings.TrimSuffix(filepath.Base(path), ".json")
	i := strings.LastIndex(base, "-")
	if i < 0 {
		return ""
	}
	return base[i+1:]
}

// --- Datei-Hilfsfunktionen --------------------------------------------------

// readJSON liest und dekodiert eine Cache-Datei.
func readJSON[T any](path string) (T, error) {
	var out T
	data, err := os.ReadFile(path)
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return out, err
	}
	return out, nil
}

// writeJSON schreibt atomar: erst in eine temporäre Datei, dann umbenennen.
// So bleibt bei einem Abbruch nie eine halbe JSON-Datei zurück, die beim
// nächsten Start einen Parse-Fehler auslöst.
func writeJSON(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*.json")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
