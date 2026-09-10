// Package vakit enthält das Domänenmodell der Gebetszeiten: die sechs
// Tagesmarken eines Diyanet-Kalendertags, die daraus abgeleiteten
// Gebetsfenster und die Kerahat-Zeiten.
//
// Das Paket ist bewusst frei von CLI-, Netzwerk- und Konfigurationsbelangen,
// damit es später auch von anderen Programmen (z. B. einem Menüleisten-Tool)
// importiert werden kann. Alle Funktionen, die "jetzt" brauchen, bekommen die
// Zeit hineingereicht statt time.Now() aufzurufen — nur so sind sie testbar.
package vakit

import (
	"fmt"
	"time"
)

// Kind bezeichnet eine der sechs Zeitmarken, die Diyanet pro Tag
// veröffentlicht. Achtung: Güneş ist kein Gebet, sondern das Ende des
// Sabah-Fensters. Siehe Kind.IsPrayerStart.
type Kind int

const (
	Imsak Kind = iota
	Gunes
	Ogle
	Ikindi
	Aksam
	Yatsi
)

// KindCount ist die Anzahl der Zeitmarken pro Tag. Wird als Array-Länge
// verwendet, damit der Compiler fehlende Marken bemerkt.
const KindCount = 6

// AllKinds listet die Zeitmarken in chronologischer Reihenfolge.
// Die Reihenfolge ist garantiert und wird von der Tagesansicht so gerendert.
var AllKinds = [KindCount]Kind{Imsak, Gunes, Ogle, Ikindi, Aksam, Yatsi}

// kindLabels sind die Anzeigenamen mit türkischer Diakritik.
var kindLabels = [KindCount]string{"İmsak", "Güneş", "Öğle", "İkindi", "Akşam", "Yatsı"}

// kindKeys sind ASCII-Bezeichner für JSON-Ausgabe, Flags und Config-Schlüssel.
// Bewusst ohne Sonderzeichen, damit `jq .vakitler[] | select(.ad=="ogle")`
// und Shell-Skripte ohne Quoting-Ärger funktionieren.
var kindKeys = [KindCount]string{"imsak", "gunes", "ogle", "ikindi", "aksam", "yatsi"}

// Label gibt den türkischen Anzeigenamen zurück ("İmsak", "Güneş", ...).
func (k Kind) Label() string { return kindLabels[k] }

// Key gibt den ASCII-Bezeichner zurück ("imsak", "gunes", ...).
func (k Kind) Key() string { return kindKeys[k] }

func (k Kind) String() string { return k.Key() }

// IsPrayerStart meldet, ob die Marke den Beginn eines Farz-Gebets bezeichnet.
// Güneş fällt heraus: es beendet das Sabah-Fenster, eröffnet aber keines.
// Diese Unterscheidung ist der Grund, warum vormittags zwischen Güneş und
// Öğle gar kein Gebetsfenster aktiv ist.
func (k Kind) IsPrayerStart() bool { return k != Gunes }

// PrayerLabel gibt den Namen des Gebets zurück, das mit dieser Marke beginnt.
// Für İmsak ist das "Sabah" — das Gebet heißt anders als die Zeitmarke.
func (k Kind) PrayerLabel() string {
	if k == Imsak {
		return "Sabah"
	}
	return k.Label()
}

// ParseKind löst einen ASCII-Bezeichner zurück in eine Kind auf.
func ParseKind(key string) (Kind, bool) {
	for i, k := range kindKeys {
		if k == key {
			return Kind(i), true
		}
	}
	return 0, false
}

// Day ist ein vollständiger Kalendertag mit allen von Diyanet gelieferten
// Angaben. Alle time.Time-Werte tragen bereits die Zeitzone des Standorts —
// das Zusammensetzen passiert im Provider, nicht hier.
type Day struct {
	// Date ist Mitternacht des Tages in der Zeitzone des Standorts.
	Date time.Time

	// Times enthält die sechs Zeitmarken, indiziert per Kind.
	Times [KindCount]time.Time

	// Sunrise und Sunset sind die *astronomischen* Zeiten (GunesDogus /
	// GunesBatis). Sie weichen von Times[Gunes] bzw. Times[Aksam] ab, weil
	// Diyanet dort eine Sicherheitsmarge (Temkin) von einigen Minuten
	// einrechnet — in Weinheim sind das 7 Minuten in beide Richtungen.
	Sunrise time.Time
	Sunset  time.Time

	// QiblaTime ist die Kıble saati: der Zeitpunkt, zu dem die Sonne genau in
	// Kıble-Richtung steht, sodass man den Gebetsteppich am Schatten
	// ausrichten kann.
	QiblaTime time.Time

	// HijriShort/HijriLong kommen unverändert aus der API ("24.3.1448" /
	// "24 Rebiulevvel 1448"). Das Hicri-Datum lässt sich nicht zuverlässig
	// selbst berechnen, weil Diyanet eigene Konventionen anwendet.
	HijriShort string
	HijriLong  string
}

// At liefert die Zeitmarke k dieses Tages.
func (d Day) At(k Kind) time.Time { return d.Times[k] }

// Weekday gibt den türkischen Wochentagsnamen zurück ("Pazar", "Pazartesi", ...).
// Wird selbst berechnet statt aus der API geparst — das ist robuster als sich
// auf das Format von MiladiTarihUzun zu verlassen.
func (d Day) Weekday() string { return TurkishWeekday(d.Date) }

// FormatDate liefert das gregorianische Datum in türkischer Langform,
// z. B. "Pazar, 6 Eylül 2026".
func (d Day) FormatDate() string {
	return fmt.Sprintf("%s, %d %s %d",
		d.Weekday(), d.Date.Day(), TurkishMonth(d.Date.Month()), d.Date.Year())
}

// turkishWeekdays folgt time.Weekday: Index 0 ist Sonntag.
var turkishWeekdays = [7]string{
	"Pazar", "Pazartesi", "Salı", "Çarşamba", "Perşembe", "Cuma", "Cumartesi",
}

// turkishMonths ist 1-basiert indiziert (Index 0 bleibt leer), damit
// time.Month direkt als Index dienen kann.
var turkishMonths = [13]string{
	"", "Ocak", "Şubat", "Mart", "Nisan", "Mayıs", "Haziran",
	"Temmuz", "Ağustos", "Eylül", "Ekim", "Kasım", "Aralık",
}

// TurkishWeekday gibt den türkischen Wochentagsnamen für t zurück.
func TurkishWeekday(t time.Time) string { return turkishWeekdays[int(t.Weekday())] }

// TurkishMonth gibt den türkischen Monatsnamen für m zurück.
func TurkishMonth(m time.Month) string { return turkishMonths[int(m)] }

// WithOffsets verschiebt die sechs Zeitmarken um die angegebenen Dauern und
// gibt eine Kopie zurück.
//
// Gedacht ist das vor allem für İkindi: Diyanet veröffentlicht die frühe
// Schattenlänge (asr-ı evvel, Faktor 1). Wer den hanefitischen asr-ı sani
// bevorzugt, verschiebt İkindi über die Config nach hinten.
//
// Sunrise, Sunset und QiblaTime bleiben unangetastet — das sind astronomische
// Messwerte, keine Konventionen. Die Kerahat-Fenster verschieben sich
// dagegen mit, weil sie aus den Vakit-Zeiten abgeleitet werden.
func (d Day) WithOffsets(offsets [KindCount]time.Duration) Day {
	out := d
	for i := range out.Times {
		out.Times[i] = out.Times[i].Add(offsets[i])
	}
	return out
}

// WithOffsets wendet WithOffsets auf jeden Tag des Kalenders an.
func (c Calendar) WithOffsets(offsets [KindCount]time.Duration) Calendar {
	// Kein Offset gesetzt: Kopie sparen.
	empty := true
	for _, o := range offsets {
		if o != 0 {
			empty = false
			break
		}
	}
	if empty {
		return c
	}

	out := Calendar{Location: c.Location, Days: make([]Day, len(c.Days))}
	for i, d := range c.Days {
		out.Days[i] = d.WithOffsets(offsets)
	}
	return out
}

// Calendar ist eine chronologisch sortierte Folge von Tagen für genau einen
// Standort — typischerweise das 32-Tage-Fenster, das die API am Stück liefert.
type Calendar struct {
	// Location ist die Zeitzone, in der alle Tage interpretiert wurden.
	Location *time.Location
	Days     []Day
}

// FindDate sucht den Tag, dessen Kalenderdatum auf date fällt. Verglichen wird
// nur Jahr/Monat/Tag in der Zeitzone des Kalenders, nicht die Uhrzeit.
func (c Calendar) FindDate(date time.Time) (Day, bool) {
	want := date.In(c.Location)
	wy, wm, wd := want.Date()
	for _, d := range c.Days {
		y, m, dd := d.Date.Date()
		if y == wy && m == wm && dd == wd {
			return d, true
		}
	}
	return Day{}, false
}

// Covers meldet, ob date im Kalender enthalten ist.
func (c Calendar) Covers(date time.Time) bool {
	_, ok := c.FindDate(date)
	return ok
}

// Range gibt den ersten und letzten enthaltenen Tag zurück. Bei leerem
// Kalender sind beide Werte die Nullzeit.
func (c Calendar) Range() (first, last time.Time) {
	if len(c.Days) == 0 {
		return time.Time{}, time.Time{}
	}
	return c.Days[0].Date, c.Days[len(c.Days)-1].Date
}
