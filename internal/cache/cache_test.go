package cache

import (
	"testing"
	"time"

	"github.com/samu380/namaz-vakti/pkg/vakit"
)

// TestRoundTripAcrossDST ist der eigentliche Grund, warum der Cache Uhrzeiten
// als "HH:MM"-Wandzeit ablegt und nicht als absoluten Zeitstempel.
//
// Das Zeitfenster der API umfasst rund 32 Tage und reicht damit regelmäßig
// über die Sommerzeitumstellung hinaus. Würden wir absolute Zeitstempel
// speichern, wären alle Tage jenseits der Umstellung um eine Stunde
// verschoben. Durch das erneute Anwenden der Zeitzone beim Laden stimmen sie.
//
// Geprüft wird die EU-Umstellung am 25. Oktober 2026: davor CEST (+02:00),
// danach CET (+01:00).
func TestRoundTripAcrossDST(t *testing.T) {
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatalf("Zeitzone: %v", err)
	}

	day := func(date string, ikindi string) vakit.Day {
		d, err := time.ParseInLocation("2006-01-02", date, berlin)
		if err != nil {
			t.Fatalf("Datum %q: %v", date, err)
		}
		c, err := time.Parse("15:04", ikindi)
		if err != nil {
			t.Fatalf("Uhrzeit %q: %v", ikindi, err)
		}
		out := vakit.Day{Date: d}
		for i := range out.Times {
			out.Times[i] = time.Date(d.Year(), d.Month(), d.Day(), c.Hour(), c.Minute(), 0, 0, berlin)
		}
		return out
	}

	cal := vakit.Calendar{
		Location: berlin,
		Days: []vakit.Day{
			day("2026-10-24", "17:06"), // noch Sommerzeit
			day("2026-10-26", "17:06"), // schon Winterzeit
		},
	}

	stored := encodeCalendar("ezanvakti", "10532", cal, time.Now())
	back, err := stored.decode(berlin)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	tests := []struct {
		idx        int
		wantOffset int // Sekunden östlich von UTC
		wantZone   string
	}{
		{0, 2 * 3600, "CEST"},
		{1, 1 * 3600, "CET"},
	}
	for _, tc := range tests {
		got := back.Days[tc.idx].At(vakit.Ikindi)
		zone, offset := got.Zone()

		if got.Format("15:04") != "17:06" {
			t.Errorf("Tag %d: Uhrzeit = %s, erwartet 17:06", tc.idx, got.Format("15:04"))
		}
		if offset != tc.wantOffset {
			t.Errorf("Tag %d: Offset = %ds, erwartet %ds", tc.idx, offset, tc.wantOffset)
		}
		if zone != tc.wantZone {
			t.Errorf("Tag %d: Zone = %s, erwartet %s", tc.idx, zone, tc.wantZone)
		}
	}
}

// TestRoundTripPreservesFields prüft, dass alle Felder den Weg durch den
// Cache überleben — inklusive der optionalen astronomischen Angaben.
func TestRoundTripPreservesFields(t *testing.T) {
	berlin, _ := time.LoadLocation("Europe/Berlin")
	date := time.Date(2026, 9, 6, 0, 0, 0, 0, berlin)

	at := func(hh, mm int) time.Time {
		return time.Date(2026, 9, 6, hh, mm, 0, 0, berlin)
	}

	orig := vakit.Day{
		Date:       date,
		Times:      [vakit.KindCount]time.Time{at(4, 53), at(6, 41), at(13, 29), at(17, 6), at(20, 7), at(21, 41)},
		Sunrise:    at(6, 48),
		Sunset:     at(20, 0),
		QiblaTime:  at(10, 40),
		HijriShort: "24.3.1448",
		HijriLong:  "24 Rebiulevvel 1448",
	}

	cal := vakit.Calendar{Location: berlin, Days: []vakit.Day{orig}}
	back, err := encodeCalendar("ezanvakti", "10532", cal, time.Now()).decode(berlin)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	got := back.Days[0]

	for _, k := range vakit.AllKinds {
		if !got.At(k).Equal(orig.At(k)) {
			t.Errorf("%s = %v, erwartet %v", k.Label(), got.At(k), orig.At(k))
		}
	}
	if !got.Sunrise.Equal(orig.Sunrise) {
		t.Errorf("Sunrise = %v, erwartet %v", got.Sunrise, orig.Sunrise)
	}
	if !got.Sunset.Equal(orig.Sunset) {
		t.Errorf("Sunset = %v, erwartet %v", got.Sunset, orig.Sunset)
	}
	if !got.QiblaTime.Equal(orig.QiblaTime) {
		t.Errorf("QiblaTime = %v, erwartet %v", got.QiblaTime, orig.QiblaTime)
	}
	if got.HijriLong != orig.HijriLong {
		t.Errorf("HijriLong = %q, erwartet %q", got.HijriLong, orig.HijriLong)
	}
}

// TestRoundTripZeroTimes prüft, dass fehlende Zusatzangaben nicht zu
// 00:00-Werten mutieren. Die API liefert einzelne Felder gelegentlich leer;
// dann soll die Nullzeit erhalten bleiben, damit die Anzeige "—" schreibt.
func TestRoundTripZeroTimes(t *testing.T) {
	berlin, _ := time.LoadLocation("Europe/Berlin")
	date := time.Date(2026, 9, 6, 0, 0, 0, 0, berlin)

	cal := vakit.Calendar{Location: berlin, Days: []vakit.Day{{Date: date}}}
	back, err := encodeCalendar("ezanvakti", "1", cal, time.Now()).decode(berlin)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !back.Days[0].Sunrise.IsZero() {
		t.Errorf("Sunrise = %v, erwartet Nullzeit", back.Days[0].Sunrise)
	}
	if !back.Days[0].QiblaTime.IsZero() {
		t.Errorf("QiblaTime = %v, erwartet Nullzeit", back.Days[0].QiblaTime)
	}
}

// TestCovers prüft, dass der Cache auch den Folgetag verlangt — das
// Yatsı-Fenster reicht bis zum İmsak des nächsten Tages.
func TestCovers(t *testing.T) {
	berlin, _ := time.LoadLocation("Europe/Berlin")
	mk := func(dates ...string) vakit.Calendar {
		cal := vakit.Calendar{Location: berlin}
		for _, d := range dates {
			parsed, _ := time.ParseInLocation("2006-01-02", d, berlin)
			cal.Days = append(cal.Days, vakit.Day{Date: parsed})
		}
		return cal
	}
	need := time.Date(2026, 9, 6, 23, 0, 0, 0, berlin)

	if covers(mk("2026-09-06"), need) {
		t.Error("nur der Tag selbst sollte nicht genügen — der Folgetag fehlt")
	}
	if !covers(mk("2026-09-06", "2026-09-07"), need) {
		t.Error("Tag und Folgetag sollten genügen")
	}
}

// TestIDFromFilename prüft die Extraktion der ID aus Cache-Dateinamen.
func TestIDFromFilename(t *testing.T) {
	tests := map[string]string{
		"/tmp/ilceler-ezanvakti-850.json":    "850",
		"/tmp/sehirler-ezanvakti-13.json":    "13",
		"/tmp/vakitler-ezanvakti-10532.json": "10532",
		"kaputt.json":                        "",
	}
	for in, want := range tests {
		if got := idFromFilename(in); got != want {
			t.Errorf("idFromFilename(%q) = %q, erwartet %q", in, got, want)
		}
	}
}
