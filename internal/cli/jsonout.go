package cli

import (
	"encoding/json"
	"io"
	"time"

	"github.com/samu380/namaz-vakti/pkg/vakit"
)

// Die JSON-Ausgabe ist als *stabile Schnittstelle* gedacht: Menüleisten-Tools,
// tmux-Statuszeilen und Skripte sollen sich darauf verlassen können.
//
// Zwei bewusste Festlegungen:
//
//   - Alle Schlüssel sind reines ASCII ("su_anki" statt "şu_anki"). Das erspart
//     Quoting-Ärger in jq-Ausdrücken und Shell-Skripten.
//   - Jede Uhrzeit erscheint doppelt: als "HH:MM" zum Anzeigen und als
//     vollständiger ISO-8601-Zeitstempel mit Zonenoffset zum Rechnen.

type jsonLocation struct {
	Key        string `json:"anahtar"`
	Name       string `json:"ad"`
	DistrictID string `json:"ilce_id"`
	TZ         string `json:"saat_dilimi"`
}

type jsonDate struct {
	Gregorian string `json:"miladi"` // "2026-09-06"
	Weekday   string `json:"gun"`    // "Pazar"
	Hijri     string `json:"hicri"`  // "24 Rebiulevvel 1448"
}

type jsonTime struct {
	Key   string `json:"ad"`     // "ikindi"
	Label string `json:"etiket"` // "İkindi"
	Clock string `json:"saat"`   // "17:06"
	ISO   string `json:"iso"`    // "2026-09-06T17:06:00+02:00"
}

type jsonKerahat struct {
	Key      string `json:"tur"`    // "isfirar"
	Label    string `json:"etiket"` // "İsfirar"
	Start    string `json:"baslangic"`
	End      string `json:"bitis"`
	StartISO string `json:"baslangic_iso"`
	EndISO   string `json:"bitis_iso"`
	Active   bool   `json:"aktif"`
	// RemainingSec ist nur bei aktiven Fenstern gesetzt.
	RemainingSec *int `json:"kalan_saniye,omitempty"`
}

type jsonCurrent struct {
	Prayer       string `json:"vakit"` // "Öğle"
	Opens        string `json:"baslangic_vakti"`
	End          string `json:"bitis"`
	EndISO       string `json:"bitis_iso"`
	RemainingSec int    `json:"kalan_saniye"`
}

type jsonNext struct {
	Key          string `json:"ad"`
	Prayer       string `json:"vakit"`
	Clock        string `json:"saat"`
	ISO          string `json:"iso"`
	RemainingSec int    `json:"kalan_saniye"`
}

type jsonSource struct {
	Provider  string `json:"saglayici"`
	FromCache bool   `json:"onbellekten"`
	FetchedAt string `json:"alindi"`
}

// jsonDay ist die Wurzelstruktur von `namaz --json`.
type jsonDay struct {
	Location jsonLocation `json:"konum"`
	Date     jsonDate     `json:"tarih"`
	Times    []jsonTime   `json:"vakitler"`

	// Astronomische Zusatzangaben. Sie weichen von Güneş/Akşam ab, weil
	// Diyanet dort eine Temkin-Marge einrechnet.
	Sunrise string `json:"gunes_dogus,omitempty"`
	Sunset  string `json:"gunes_batis,omitempty"`
	Qibla   string `json:"kible_saati,omitempty"`

	// Current und Next sind nur bei der Ansicht für *heute* gesetzt.
	Current *jsonCurrent `json:"su_anki"`
	Next    *jsonNext    `json:"sonraki"`

	Kerahat []jsonKerahat `json:"kerahat"`
	Source  jsonSource    `json:"kaynak"`
}

// iso formatiert eine Zeit als RFC 3339. Nullzeiten werden zu "".
func iso(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

// buildTimes übersetzt die sechs Zeitmarken.
func buildTimes(d vakit.Day) []jsonTime {
	out := make([]jsonTime, 0, vakit.KindCount)
	for _, k := range vakit.AllKinds {
		out = append(out, jsonTime{
			Key:   k.Key(),
			Label: k.Label(),
			Clock: hhmm(d.At(k)),
			ISO:   iso(d.At(k)),
		})
	}
	return out
}

// buildKerahats übersetzt die drei Kerahat-Fenster. now bestimmt, welches als
// aktiv markiert wird.
func buildKerahats(d vakit.Day, dur vakit.KerahatDurations, now time.Time, withNow bool) []jsonKerahat {
	ks := d.Kerahats(dur)
	out := make([]jsonKerahat, 0, len(ks))
	for _, k := range ks {
		entry := jsonKerahat{
			Key:      k.Kind.Key(),
			Label:    k.Kind.Label(),
			Start:    hhmm(k.Start),
			End:      hhmm(k.End),
			StartISO: iso(k.Start),
			EndISO:   iso(k.End),
		}
		if withNow && k.Contains(now) {
			entry.Active = true
			sec := int(k.End.Sub(now).Seconds())
			entry.RemainingSec = &sec
		}
		out = append(out, entry)
	}
	return out
}

// writeJSON gibt v eingerückt aus. Eingerückt statt kompakt, weil die Ausgabe
// oft direkt gelesen wird; jq und Skripte stört das nicht.
func writeJSONOut(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false) // sonst würden türkische Zeichen entstellt
	return enc.Encode(v)
}
