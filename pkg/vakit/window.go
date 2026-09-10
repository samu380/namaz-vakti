package vakit

import (
	"errors"
	"time"
)

// ErrDateNotCovered meldet, dass der benötigte Tag nicht im Kalender liegt.
// Der Aufrufer muss dann neu von der API laden.
var ErrDateNotCovered = errors.New("tarih takvimde yok")

// Window ist das Gültigkeitsfenster eines Farz-Gebets: der Zeitraum, in dem
// das Gebet verrichtet werden darf.
type Window struct {
	// Opens ist die Zeitmarke, die das Fenster eröffnet. Für Sabah ist das
	// İmsak — das Gebet heißt also anders als seine Startmarke.
	Opens Kind

	// Prayer ist der Anzeigename des Gebets ("Sabah", "Öğle", ...).
	Prayer string

	From time.Time
	To   time.Time
}

// Contains meldet, ob t im Fenster liegt (Beginn inklusive, Ende exklusiv).
func (w Window) Contains(t time.Time) bool {
	return !t.Before(w.From) && t.Before(w.To)
}

// Windows baut die fünf Farz-Fenster eines Tages.
//
// Vier der fünf Fenster enden genau dann, wenn das nächste Gebet beginnt.
// Sabah ist die Ausnahme: es endet mit Güneş, das nächste Gebet (Öğle) folgt
// aber erst Stunden später. Dazwischen liegt eine Lücke ohne aktives Fenster.
//
// Das Yatsı-Fenster reicht bis zum İmsak des Folgetags, deshalb wird next
// benötigt. Fehlt der Folgetag, endet Yatsı ersatzweise um Mitternacht.
func (d Day) Windows(next *Day) []Window {
	yatsiEnd := d.Date.AddDate(0, 0, 1) // Fallback: Mitternacht
	if next != nil {
		yatsiEnd = next.At(Imsak)
	}
	return []Window{
		{Opens: Imsak, Prayer: "Sabah", From: d.At(Imsak), To: d.At(Gunes)},
		{Opens: Ogle, Prayer: "Öğle", From: d.At(Ogle), To: d.At(Ikindi)},
		{Opens: Ikindi, Prayer: "İkindi", From: d.At(Ikindi), To: d.At(Aksam)},
		{Opens: Aksam, Prayer: "Akşam", From: d.At(Aksam), To: d.At(Yatsi)},
		{Opens: Yatsi, Prayer: "Yatsı", From: d.At(Yatsi), To: yatsiEnd},
	}
}

// Event ist ein bevorstehender Gebetsbeginn.
type Event struct {
	Kind   Kind
	Prayer string
	At     time.Time
}

// Status ist die Momentaufnahme, aus der die Tagesansicht gerendert wird.
type Status struct {
	// Now ist der Bezugszeitpunkt, bereits in der Zeitzone des Standorts.
	Now time.Time

	// Today ist der Kalendertag, dessen Tabelle angezeigt wird.
	Today Day

	// Current ist das gerade laufende Farz-Fenster, oder nil. Nil bedeutet
	// nicht "Fehler", sondern die Vormittagslücke zwischen Güneş und Öğle.
	Current *Window

	// Next ist der nächste Gebetsbeginn. Güneş wird übersprungen, weil es
	// kein Gebet eröffnet.
	Next *Event

	// Kerahat ist das gerade laufende Kerahat-Fenster, oder nil.
	Kerahat *Kerahat
}

// Remaining gibt die Restzeit im laufenden Fenster zurück. Ist kein Fenster
// aktiv, ist das Ergebnis 0 und ok false.
func (s Status) Remaining() (d time.Duration, ok bool) {
	if s.Current == nil {
		return 0, false
	}
	return s.Current.To.Sub(s.Now), true
}

// UntilNext gibt die Zeit bis zum nächsten Gebetsbeginn zurück.
func (s Status) UntilNext() (d time.Duration, ok bool) {
	if s.Next == nil {
		return 0, false
	}
	return s.Next.At.Sub(s.Now), true
}

// NextIsWindowEnd meldet, ob das laufende Fenster genau dann endet, wenn das
// nächste Gebet beginnt.
//
// Das ist der Normalfall (Öğle→İkindi, İkindi→Akşam, Akşam→Yatsı, Yatsı→Sabah)
// und der Grund, warum "Restzeit" und "Countdown" meist dieselbe Zahl sind.
// Die Tagesansicht fasst beide dann zu einer Zeile zusammen, statt dieselbe
// Zahl doppelt anzuzeigen. Nur im Sabah-Fenster fallen sie auseinander.
func (s Status) NextIsWindowEnd() bool {
	return s.Current != nil && s.Next != nil && s.Current.To.Equal(s.Next.At)
}

// KerahatRemaining gibt die Restzeit im laufenden Kerahat-Fenster zurück.
func (s Status) KerahatRemaining() (d time.Duration, ok bool) {
	if s.Kerahat == nil {
		return 0, false
	}
	return s.Kerahat.End.Sub(s.Now), true
}

// StatusAt berechnet den Status zum Zeitpunkt now.
//
// Für die Fensterberechnung werden Vortag, Tag und Folgetag herangezogen:
// Um 03:00 läuft noch das Yatsı-Fenster des Vortags, und das Yatsı-Fenster
// des aktuellen Tages endet erst am Folgetag. Fehlt einer dieser Tage im
// Kalender, wird ErrDateNotCovered zurückgegeben, damit der Aufrufer
// nachladen kann.
func (c Calendar) StatusAt(now time.Time, dur KerahatDurations) (Status, error) {
	now = now.In(c.Location)

	today, ok := c.FindDate(now)
	if !ok {
		return Status{}, ErrDateNotCovered
	}

	// Nachbartage sind optional: fehlen sie, arbeiten wir mit dem, was da ist.
	// Nur der heutige Tag ist zwingend.
	yesterday, hasYesterday := c.FindDate(now.AddDate(0, 0, -1))
	tomorrow, hasTomorrow := c.FindDate(now.AddDate(0, 0, 1))

	var windows []Window
	if hasYesterday {
		// Der Vortag braucht seinerseits den heutigen Tag, damit sein
		// Yatsı-Fenster bis zum heutigen İmsak reicht.
		windows = append(windows, yesterday.Windows(&today)...)
	}
	if hasTomorrow {
		windows = append(windows, today.Windows(&tomorrow)...)
		windows = append(windows, tomorrow.Windows(nil)...)
	} else {
		windows = append(windows, today.Windows(nil)...)
	}

	st := Status{Now: now, Today: today}

	// Laufendes Fenster suchen.
	for i := range windows {
		if windows[i].Contains(now) {
			st.Current = &windows[i]
			break
		}
	}

	// Nächsten Gebetsbeginn suchen: das früheste Fenster, das nach now
	// beginnt. Da windows chronologisch pro Tag aufgebaut ist und die Tage in
	// Reihenfolge angehängt wurden, ist der erste Treffer der richtige.
	for i := range windows {
		if windows[i].From.After(now) {
			st.Next = &Event{
				Kind:   windows[i].Opens,
				Prayer: windows[i].Prayer,
				At:     windows[i].From,
			}
			break
		}
	}

	// Kerahat über die Tagesgrenze hinweg prüfen. Alle drei Fenster liegen
	// zwar tagsüber, aber die Prüfung über drei Tage kostet nichts und
	// vermeidet Randfälle bei Zeitumstellungen.
	days := []Day{today}
	if hasYesterday {
		days = append(days, yesterday)
	}
	if hasTomorrow {
		days = append(days, tomorrow)
	}
	for _, d := range days {
		if k := d.ActiveKerahat(now, dur); k != nil {
			st.Kerahat = k
			break
		}
	}

	return st, nil
}
