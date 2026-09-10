package vakit

import "time"

// KerahatKind bezeichnet eines der drei Zeitfenster, in denen das Gebet nach
// hanefitischer Auffassung mekruh (unerwünscht) ist.
type KerahatKind int

const (
	// Israk beginnt mit dem Sonnenaufgang und dauert an, bis die Sonne sich
	// ein Stück über den Horizont gehoben hat.
	Israk KerahatKind = iota

	// Istiva ist der Zeitraum um den Sonnenhöchststand, unmittelbar vor Öğle.
	Istiva

	// Isfirar beginnt, wenn sich die Sonne zum Untergang hin gelb färbt, und
	// endet mit Akşam. Das İkindi-Gebet ist darin noch gültig, aber mekruh —
	// es ist damit das einzige Kerahat-Fenster, das in ein Gebetsfenster
	// hineinragt.
	Isfirar
)

var kerahatLabels = map[KerahatKind]string{
	Israk:   "İşrak",
	Istiva:  "İstiva",
	Isfirar: "İsfirar",
}

// kerahatKeys sind ASCII-Bezeichner für JSON und Config-Schlüssel.
var kerahatKeys = map[KerahatKind]string{
	Israk:   "israk",
	Istiva:  "istiva",
	Isfirar: "isfirar",
}

// kerahatNotes erklären in einem Halbsatz, woran das Fenster hängt.
var kerahatNotes = map[KerahatKind]string{
	Israk:   "güneş doğduktan sonra",
	Istiva:  "öğleden önce",
	Isfirar: "akşamdan önce",
}

func (k KerahatKind) Label() string  { return kerahatLabels[k] }
func (k KerahatKind) Key() string    { return kerahatKeys[k] }
func (k KerahatKind) Note() string   { return kerahatNotes[k] }
func (k KerahatKind) String() string { return k.Key() }

// AllKerahatKinds listet die Fenster in chronologischer Tagesreihenfolge.
var AllKerahatKinds = []KerahatKind{Israk, Istiva, Isfirar}

// KerahatDurations legt fest, wie lang die drei Fenster angesetzt werden.
// Die API liefert keine Kerahat-Zeiten, deshalb leiten wir sie aus den
// veröffentlichten Vakit-Zeiten ab. 45 Minuten entspricht der gängigen
// türkischen Kalenderkonvention; über die Config ist jedes Fenster einzeln
// anpassbar.
type KerahatDurations struct {
	Israk   time.Duration
	Istiva  time.Duration
	Isfirar time.Duration
}

// DefaultKerahatDurations sind die Vorgabewerte (je 45 Minuten).
func DefaultKerahatDurations() KerahatDurations {
	return KerahatDurations{
		Israk:   45 * time.Minute,
		Istiva:  45 * time.Minute,
		Isfirar: 45 * time.Minute,
	}
}

// Kerahat ist ein konkretes Kerahat-Fenster an einem konkreten Tag.
type Kerahat struct {
	Kind  KerahatKind
	Start time.Time
	End   time.Time
}

// Contains meldet, ob t innerhalb des Fensters liegt. Der Beginn zählt dazu,
// das Ende nicht — sonst wäre man im Moment von Öğle gleichzeitig in der
// İstiva-Kerahat und im Öğle-Fenster.
func (k Kerahat) Contains(t time.Time) bool {
	return !t.Before(k.Start) && t.Before(k.End)
}

// Kerahats berechnet die drei Fenster des Tages.
//
// Die Fenster werden bewusst an den *veröffentlichten* Diyanet-Zeiten
// verankert (Güneş, Öğle, Akşam) und nicht an den astronomischen
// (GunesDogus/GunesBatis). Damit erben sie Diyanets Temkin-Marge, was zu den
// gedruckten türkischen Kalendern passt.
func (d Day) Kerahats(dur KerahatDurations) []Kerahat {
	return []Kerahat{
		// Ab Sonnenaufgang, bis die Sonne merklich gestiegen ist.
		{Kind: Israk, Start: d.At(Gunes), End: d.At(Gunes).Add(dur.Israk)},
		// Vor dem Mittagsgebet, endet exakt mit Öğle.
		{Kind: Istiva, Start: d.At(Ogle).Add(-dur.Istiva), End: d.At(Ogle)},
		// Vor dem Abendgebet, endet exakt mit Akşam.
		{Kind: Isfirar, Start: d.At(Aksam).Add(-dur.Isfirar), End: d.At(Aksam)},
	}
}

// ActiveKerahat gibt das Fenster zurück, in dem t liegt, oder nil.
func (d Day) ActiveKerahat(t time.Time, dur KerahatDurations) *Kerahat {
	for _, k := range d.Kerahats(dur) {
		if k.Contains(t) {
			return &k
		}
	}
	return nil
}
