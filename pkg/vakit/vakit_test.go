package vakit

import (
	"testing"
	"time"
)

// Die Tests arbeiten mit echten Diyanet-Daten für Weinheim (İlçe 10532),
// abgerufen am 7. September 2026. Damit prüfen sie nicht nur die Logik,
// sondern auch, dass die Annahmen über Diyanets Zeiten stimmen — etwa die
// Temkin-Marge von 7 Minuten zwischen Güneş und dem astronomischen
// Sonnenaufgang.

var berlin = mustLoadLocation("Europe/Berlin")

func mustLoadLocation(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return loc
}

// fixtureDay baut einen Tag aus "HH:MM"-Angaben.
func fixtureDay(t *testing.T, date string, times [KindCount]string, sunrise, sunset string) Day {
	t.Helper()

	d, err := time.ParseInLocation("2006-01-02", date, berlin)
	if err != nil {
		t.Fatalf("Fixture-Datum %q: %v", date, err)
	}
	at := func(hhmm string) time.Time {
		c, err := time.Parse("15:04", hhmm)
		if err != nil {
			t.Fatalf("Fixture-Uhrzeit %q: %v", hhmm, err)
		}
		return time.Date(d.Year(), d.Month(), d.Day(), c.Hour(), c.Minute(), 0, 0, berlin)
	}

	day := Day{Date: d, Sunrise: at(sunrise), Sunset: at(sunset)}
	for i, hhmm := range times {
		day.Times[i] = at(hhmm)
	}
	return day
}

// weinheim liefert den Kalender vom 5. bis 8. September 2026.
// Reihenfolge der Zeiten: İmsak, Güneş, Öğle, İkindi, Akşam, Yatsı.
func weinheim(t *testing.T) Calendar {
	t.Helper()
	return Calendar{
		Location: berlin,
		Days: []Day{
			fixtureDay(t, "2026-09-05", [KindCount]string{"04:51", "06:39", "13:29", "17:08", "20:09", "21:43"}, "06:46", "20:02"),
			fixtureDay(t, "2026-09-06", [KindCount]string{"04:53", "06:41", "13:29", "17:06", "20:07", "21:41"}, "06:48", "20:00"),
			fixtureDay(t, "2026-09-07", [KindCount]string{"04:55", "06:42", "13:29", "17:05", "20:05", "21:38"}, "06:49", "19:58"),
			fixtureDay(t, "2026-09-08", [KindCount]string{"04:57", "06:44", "13:28", "17:04", "20:03", "21:35"}, "06:51", "19:56"),
		},
	}
}

// at ist eine Kurzform für einen Zeitpunkt am 6. September 2026.
func at(hh, mm int) time.Time {
	return time.Date(2026, 9, 6, hh, mm, 0, 0, berlin)
}

// TestTemkin belegt die Annahme, auf der die Kerahat-Ableitung ruht: Diyanet
// veröffentlicht Güneş einige Minuten *vor* dem astronomischen Sonnenaufgang
// und Akşam einige Minuten *danach*.
func TestTemkin(t *testing.T) {
	day := weinheim(t).Days[1] // 2026-09-06

	if got := day.Sunrise.Sub(day.At(Gunes)); got != 7*time.Minute {
		t.Errorf("Temkin vor Güneş = %v, erwartet 7m", got)
	}
	if got := day.At(Aksam).Sub(day.Sunset); got != 7*time.Minute {
		t.Errorf("Temkin nach Akşam = %v, erwartet 7m", got)
	}
}

// TestStatusAt prüft die Countdown-Logik über den ganzen Tag.
//
// Der interessante Teil ist wantCoincide: es sagt, ob das laufende Fenster
// genau dann endet, wenn das nächste Gebet beginnt. Nur wenn das *nicht* so
// ist, zeigt die Tagesansicht eine zweite Zeile. Erwartet wird, dass das
// ausschließlich im Sabah-Fenster passiert.
func TestStatusAt(t *testing.T) {
	cal := weinheim(t)
	dur := DefaultKerahatDurations()

	tests := []struct {
		name          string
		now           time.Time
		wantCurrent   string // "" = kein Fenster aktiv
		wantEnd       string
		wantNext      string
		wantNextAt    string
		wantCoincide  bool
		wantRemaining time.Duration
	}{
		{
			// Nachts läuft noch das Yatsı-Fenster des Vortags, es endet mit
			// dem heutigen İmsak.
			name: "03:00 Yatsı vom Vortag", now: at(3, 0),
			wantCurrent: "Yatsı", wantEnd: "04:53",
			wantNext: "Sabah", wantNextAt: "04:53",
			wantCoincide: true, wantRemaining: 1*time.Hour + 53*time.Minute,
		},
		{
			// Der einzige Fall mit zwei verschiedenen Zahlen: das
			// Sabah-Fenster endet mit Güneş um 06:41, das nächste Gebet
			// (Öğle) beginnt aber erst um 13:29.
			name: "05:30 Sabah — Zahlen weichen ab", now: at(5, 30),
			wantCurrent: "Sabah", wantEnd: "06:41",
			wantNext: "Öğle", wantNextAt: "13:29",
			wantCoincide: false, wantRemaining: 1*time.Hour + 11*time.Minute,
		},
		{
			// Vormittagslücke: Sabah ist vorbei, Öğle noch nicht da.
			name: "09:00 kein Fenster", now: at(9, 0),
			wantCurrent: "",
			wantNext:    "Öğle", wantNextAt: "13:29",
		},
		{
			// İstiva-Kerahat läuft, aber ein Gebetsfenster gibt es nicht.
			name: "13:00 İstiva, kein Fenster", now: at(13, 0),
			wantCurrent: "",
			wantNext:    "Öğle", wantNextAt: "13:29",
		},
		{
			name: "14:12 Öğle", now: at(14, 12),
			wantCurrent: "Öğle", wantEnd: "17:06",
			wantNext: "İkindi", wantNextAt: "17:06",
			wantCoincide: true, wantRemaining: 2*time.Hour + 54*time.Minute,
		},
		{
			name: "18:00 İkindi", now: at(18, 0),
			wantCurrent: "İkindi", wantEnd: "20:07",
			wantNext: "Akşam", wantNextAt: "20:07",
			wantCoincide: true, wantRemaining: 2*time.Hour + 7*time.Minute,
		},
		{
			name: "20:30 Akşam", now: at(20, 30),
			wantCurrent: "Akşam", wantEnd: "21:41",
			wantNext: "Yatsı", wantNextAt: "21:41",
			wantCoincide: true, wantRemaining: 1*time.Hour + 11*time.Minute,
		},
		{
			// Yatsı reicht über Mitternacht bis zum İmsak des Folgetags.
			name: "23:00 Yatsı über Mitternacht", now: at(23, 0),
			wantCurrent: "Yatsı", wantEnd: "04:55",
			wantNext: "Sabah", wantNextAt: "04:55",
			wantCoincide: true, wantRemaining: 5*time.Hour + 55*time.Minute,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			st, err := cal.StatusAt(tc.now, dur)
			if err != nil {
				t.Fatalf("StatusAt: %v", err)
			}

			if tc.wantCurrent == "" {
				if st.Current != nil {
					t.Errorf("Current = %q, erwartet keins", st.Current.Prayer)
				}
			} else {
				if st.Current == nil {
					t.Fatalf("Current = nil, erwartet %q", tc.wantCurrent)
				}
				if st.Current.Prayer != tc.wantCurrent {
					t.Errorf("Current = %q, erwartet %q", st.Current.Prayer, tc.wantCurrent)
				}
				if got := st.Current.To.Format("15:04"); got != tc.wantEnd {
					t.Errorf("Fensterende = %s, erwartet %s", got, tc.wantEnd)
				}
				if rem, _ := st.Remaining(); rem != tc.wantRemaining {
					t.Errorf("Remaining = %v, erwartet %v", rem, tc.wantRemaining)
				}
			}

			if st.Next == nil {
				t.Fatalf("Next = nil, erwartet %q", tc.wantNext)
			}
			if st.Next.Prayer != tc.wantNext {
				t.Errorf("Next = %q, erwartet %q", st.Next.Prayer, tc.wantNext)
			}
			if got := st.Next.At.Format("15:04"); got != tc.wantNextAt {
				t.Errorf("Next.At = %s, erwartet %s", got, tc.wantNextAt)
			}
			if got := st.NextIsWindowEnd(); got != tc.wantCoincide {
				t.Errorf("NextIsWindowEnd = %t, erwartet %t", got, tc.wantCoincide)
			}
		})
	}
}

// TestNextSkipsGunes stellt sicher, dass Güneş nie als "nächstes Gebet"
// auftaucht — es beendet das Sabah-Fenster, eröffnet aber keines.
func TestNextSkipsGunes(t *testing.T) {
	cal := weinheim(t)

	// Um 06:00 wäre Güneş (06:41) die nächste Zeitmarke überhaupt.
	st, err := cal.StatusAt(at(6, 0), DefaultKerahatDurations())
	if err != nil {
		t.Fatalf("StatusAt: %v", err)
	}
	if st.Next.Kind == Gunes {
		t.Error("Next zeigt auf Güneş, erwartet das nächste echte Gebet")
	}
	if st.Next.Prayer != "Öğle" {
		t.Errorf("Next = %q, erwartet Öğle", st.Next.Prayer)
	}
}

// TestKerahats prüft die Ableitung der drei Fenster aus den Vakit-Zeiten.
func TestKerahats(t *testing.T) {
	day := weinheim(t).Days[1] // 2026-09-06
	ks := day.Kerahats(DefaultKerahatDurations())

	want := []struct {
		kind       KerahatKind
		start, end string
	}{
		{Israk, "06:41", "07:26"},   // Güneş → +45m
		{Istiva, "12:44", "13:29"},  // Öğle −45m → Öğle
		{Isfirar, "19:22", "20:07"}, // Akşam −45m → Akşam
	}

	if len(ks) != len(want) {
		t.Fatalf("%d Kerahat-Fenster, erwartet %d", len(ks), len(want))
	}
	for i, w := range want {
		if ks[i].Kind != w.kind {
			t.Errorf("[%d] Kind = %v, erwartet %v", i, ks[i].Kind, w.kind)
		}
		if got := ks[i].Start.Format("15:04"); got != w.start {
			t.Errorf("[%d] %v Start = %s, erwartet %s", i, w.kind, got, w.start)
		}
		if got := ks[i].End.Format("15:04"); got != w.end {
			t.Errorf("[%d] %v Ende = %s, erwartet %s", i, w.kind, got, w.end)
		}
	}
}

// TestActiveKerahat prüft die Grenzen: Beginn zählt dazu, Ende nicht.
func TestActiveKerahat(t *testing.T) {
	day := weinheim(t).Days[1]
	dur := DefaultKerahatDurations()

	tests := []struct {
		now  time.Time
		want string // "" = keine
	}{
		{at(6, 40), ""},      // eine Minute vor İşrak
		{at(6, 41), "İşrak"}, // Beginn zählt dazu
		{at(7, 25), "İşrak"},
		{at(7, 26), ""}, // Ende zählt nicht mehr dazu
		{at(12, 43), ""},
		{at(12, 44), "İstiva"},
		{at(13, 29), ""}, // endet exakt mit Öğle
		{at(19, 22), "İsfirar"},
		{at(19, 56), "İsfirar"},
		{at(20, 7), ""}, // endet exakt mit Akşam
	}

	for _, tc := range tests {
		got := ""
		if k := day.ActiveKerahat(tc.now, dur); k != nil {
			got = k.Kind.Label()
		}
		if got != tc.want {
			t.Errorf("ActiveKerahat(%s) = %q, erwartet %q", tc.now.Format("15:04"), got, tc.want)
		}
	}
}

// TestIsfirarInIkindiWindow belegt die fachliche Besonderheit, auf der die
// Option kerahat.uyari beruht: İsfirar ist das einzige Kerahat-Fenster, das
// in ein Gebetsfenster hineinragt. Die "harte" İkindi-Deadline ist Akşam,
// die "weiche" der Beginn von İsfirar.
func TestIsfirarInIkindiWindow(t *testing.T) {
	cal := weinheim(t)
	dur := DefaultKerahatDurations()

	st, err := cal.StatusAt(at(19, 56), dur)
	if err != nil {
		t.Fatalf("StatusAt: %v", err)
	}
	if st.Current == nil || st.Current.Prayer != "İkindi" {
		t.Fatalf("erwartet laufendes İkindi-Fenster, bekam %v", st.Current)
	}
	if st.Kerahat == nil || st.Kerahat.Kind != Isfirar {
		t.Fatalf("erwartet laufende İsfirar-Kerahat, bekam %v", st.Kerahat)
	}
	if rem, _ := st.KerahatRemaining(); rem != 11*time.Minute {
		t.Errorf("Kerahat-Restzeit = %v, erwartet 11m", rem)
	}
	// Weiche Deadline liegt vor der harten.
	if !st.Kerahat.Start.Before(st.Current.To) {
		t.Error("İsfirar-Beginn sollte vor dem Ende des İkindi-Fensters liegen")
	}
}

// TestWithOffsets prüft die Minutenkorrektur — der Weg zum hanefitischen
// asr-ı sani, ohne eigene Astronomie.
func TestWithOffsets(t *testing.T) {
	cal := weinheim(t)

	var offsets [KindCount]time.Duration
	offsets[Ikindi] = 52 * time.Minute
	shifted := cal.WithOffsets(offsets)

	day := shifted.Days[1]
	if got := day.At(Ikindi).Format("15:04"); got != "17:58" {
		t.Errorf("İkindi mit +52m = %s, erwartet 17:58", got)
	}
	// Unbeteiligte Zeiten bleiben unverändert.
	if got := day.At(Aksam).Format("15:04"); got != "20:07" {
		t.Errorf("Akşam = %s, erwartet unverändert 20:07", got)
	}
	// Astronomische Werte werden nicht verschoben.
	if got := day.Sunset.Format("15:04"); got != "20:00" {
		t.Errorf("Sunset = %s, erwartet unverändert 20:00", got)
	}
	// Das Öğle-Fenster endet nun später, weil İkindi später beginnt.
	st, err := shifted.StatusAt(at(17, 30), DefaultKerahatDurations())
	if err != nil {
		t.Fatalf("StatusAt: %v", err)
	}
	if st.Current == nil || st.Current.Prayer != "Öğle" {
		t.Errorf("um 17:30 erwartet Öğle-Fenster, bekam %v", st.Current)
	}

	// Ohne Offsets muss dieselbe Instanz zurückkommen (keine Kopie).
	var none [KindCount]time.Duration
	if got := cal.WithOffsets(none); &got.Days[0] != &cal.Days[0] {
		t.Error("WithOffsets ohne Korrektur sollte den Kalender unverändert durchreichen")
	}
}

// TestDateNotCovered prüft die Fehlermeldung für Tage außerhalb des Kalenders.
func TestDateNotCovered(t *testing.T) {
	cal := weinheim(t)
	_, err := cal.StatusAt(time.Date(2026, 12, 24, 12, 0, 0, 0, berlin), DefaultKerahatDurations())
	if err != ErrDateNotCovered {
		t.Errorf("Fehler = %v, erwartet ErrDateNotCovered", err)
	}
}

// TestTurkishDateFormat prüft Wochentag und Monat.
func TestTurkishDateFormat(t *testing.T) {
	day := weinheim(t).Days[1] // 6. September 2026 ist ein Sonntag
	if got := day.Weekday(); got != "Pazar" {
		t.Errorf("Weekday = %q, erwartet Pazar", got)
	}
	if got := day.FormatDate(); got != "Pazar, 6 Eylül 2026" {
		t.Errorf("FormatDate = %q, erwartet \"Pazar, 6 Eylül 2026\"", got)
	}
}

// TestKindLabels stellt sicher, dass Bezeichner und Anzeigenamen
// zusammenpassen und Güneş korrekt als Nicht-Gebet markiert ist.
func TestKindLabels(t *testing.T) {
	if Imsak.PrayerLabel() != "Sabah" {
		t.Errorf("İmsak eröffnet das Gebet %q, erwartet Sabah", Imsak.PrayerLabel())
	}
	if Gunes.IsPrayerStart() {
		t.Error("Güneş sollte kein Gebet eröffnen")
	}
	for _, k := range AllKinds {
		got, ok := ParseKind(k.Key())
		if !ok || got != k {
			t.Errorf("ParseKind(%q) = %v, %t", k.Key(), got, ok)
		}
	}
}
