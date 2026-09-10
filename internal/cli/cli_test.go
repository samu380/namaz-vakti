package cli

import (
	"reflect"
	"testing"
	"time"
)

// TestFoldTurkish prüft die Faltung türkischer Sonderzeichen.
//
// Der Fall, auf den es ankommt: strings.ToLower("İ") liefert in Go ein "i"
// mit kombinierendem Punkt, das keinem ASCII-"i" gleicht. Ohne eigene
// Faltung würde "weinheim" den API-Eintrag "WEİNHEİM" nicht finden.
func TestFoldTurkish(t *testing.T) {
	tests := map[string]string{
		"WEİNHEİM":          "weinheim",
		"weinheim":          "weinheim",
		"İstanbul":          "istanbul",
		"ISTANBUL":          "istanbul",
		"Ağrı":              "agri",
		"ÇANKIRI":           "cankiri",
		"Şanlıurfa":         "sanliurfa",
		"yarın":             "yarin",
		"sürüm":             "surum",
		"göster":            "goster",
		"varsayılan":        "varsayilan",
		"BADEN WURTTEMBERG": "baden wurttemberg",
	}
	for in, want := range tests {
		if got := foldTurkish(in); got != want {
			t.Errorf("foldTurkish(%q) = %q, erwartet %q", in, got, want)
		}
	}
}

// TestContainsFolded prüft die Teilstringsuche über die Faltung.
func TestContainsFolded(t *testing.T) {
	cases := []struct {
		haystack, needle string
		want             bool
	}{
		{"WEİNHEİM", "weinheim", true},
		{"WEİNHEİM", "wein", true},
		{"WEİNHEİM", "heim", true},
		{"MANNHEIM", "weinheim", false},
		{"KIRIKKALE", "kirik", true},
		{"Şırnak", "sirnak", true},
	}
	for _, c := range cases {
		if got := containsFolded(c.haystack, c.needle); got != c.want {
			t.Errorf("containsFolded(%q, %q) = %t, erwartet %t", c.haystack, c.needle, got, c.want)
		}
	}
}

// TestFormatDuration prüft die türkische Dauerausgabe.
func TestFormatDuration(t *testing.T) {
	cases := []struct {
		d       time.Duration
		seconds bool
		want    string
	}{
		{2*time.Hour + 54*time.Minute, false, "2 sa 54 dk"},
		{2*time.Hour + 54*time.Minute, true, "2 sa 54 dk 0 sn"},
		{54 * time.Minute, false, "54 dk"},
		{54*time.Minute + 12*time.Second, true, "54 dk 12 sn"},
		{11 * time.Minute, false, "11 dk"},
		{40 * time.Second, false, "1 dk'dan az"},
		{40 * time.Second, true, "40 sn"},
		{0, false, "1 dk'dan az"},
		// Negative Werte entstehen an Rundungsrändern und werden geklemmt.
		{-5 * time.Minute, false, "1 dk'dan az"},
	}
	for _, c := range cases {
		if got := FormatDuration(c.d, c.seconds); got != c.want {
			t.Errorf("FormatDuration(%v, %t) = %q, erwartet %q", c.d, c.seconds, got, c.want)
		}
	}
}

// TestFormatDurationShort prüft das kompakte Format für Statusleisten.
func TestFormatDurationShort(t *testing.T) {
	cases := map[time.Duration]string{
		2*time.Hour + 54*time.Minute: "2:54",
		54 * time.Minute:             "0:54",
		9 * time.Minute:              "0:09",
		0:                            "0:00",
		-time.Minute:                 "0:00",
	}
	for d, want := range cases {
		if got := formatDurationShort(d); got != want {
			t.Errorf("formatDurationShort(%v) = %q, erwartet %q", d, got, want)
		}
	}
}

// TestSplitLeadingArgs prüft die Vorsortierung, die Flags hinter
// Positionsargumenten möglich macht.
//
// Ohne sie bricht flag.Parse bei `konum ekle ev --ilce 10532` am Argument
// "ev" ab und --ilce bliebe leer.
func TestSplitLeadingArgs(t *testing.T) {
	cases := []struct {
		in        []string
		wantPos   []string
		wantFlags []string
	}{
		{
			in:      []string{"ev", "--ilce", "10532"},
			wantPos: []string{"ev"}, wantFlags: []string{"--ilce", "10532"},
		},
		{
			// Beginnt es mit einem Flag, gibt es keine führenden
			// Positionsargumente — "ev" holt flag.Parse danach aus Args().
			in:      []string{"--ilce", "10532", "ev"},
			wantPos: []string{}, wantFlags: []string{"--ilce", "10532", "ev"},
		},
		{
			in:      []string{"2026-12-24"},
			wantPos: []string{"2026-12-24"}, wantFlags: nil,
		},
		{
			in:      []string{},
			wantPos: []string{}, wantFlags: nil,
		},
		{
			// Ein einzelnes "-" ist ein Positionsargument, kein Flag.
			in:      []string{"-"},
			wantPos: []string{"-"}, wantFlags: nil,
		},
		{
			in:      []string{"ara", "weinheim", "--ulke", "ALMANYA"},
			wantPos: []string{"ara", "weinheim"}, wantFlags: []string{"--ulke", "ALMANYA"},
		},
	}

	for _, c := range cases {
		pos, flags := splitLeadingArgs(c.in)
		if !reflect.DeepEqual(pos, c.wantPos) {
			t.Errorf("splitLeadingArgs(%v) Positionsargumente = %v, erwartet %v", c.in, pos, c.wantPos)
		}
		if !reflect.DeepEqual(flags, c.wantFlags) {
			t.Errorf("splitLeadingArgs(%v) Flags = %v, erwartet %v", c.in, flags, c.wantFlags)
		}
	}
}

// TestPadding prüft, dass die Ausrichtung Zeichen und nicht Bytes zählt.
// "İmsak" belegt 6 Bytes, aber nur 5 Spalten.
func TestPadding(t *testing.T) {
	if got := runeLen("İmsak"); got != 5 {
		t.Errorf("runeLen(\"İmsak\") = %d, erwartet 5", got)
	}
	if got := runeLen("Yatsı"); got != 5 {
		t.Errorf("runeLen(\"Yatsı\") = %d, erwartet 5", got)
	}
	if got := padRight("İmsak", 9); runeLen(got) != 9 {
		t.Errorf("padRight(\"İmsak\", 9) hat %d Zeichen, erwartet 9", runeLen(got))
	}
	if got := padLeft("Öğle", 8); runeLen(got) != 8 {
		t.Errorf("padLeft(\"Öğle\", 8) hat %d Zeichen, erwartet 8", runeLen(got))
	}
	// Längere Eingaben werden nicht abgeschnitten.
	if got := padRight("Çarşamba", 4); got != "Çarşamba" {
		t.Errorf("padRight kürzt: %q", got)
	}
}

// TestStyleOff stellt sicher, dass ohne Farbe keine Steuerzeichen entstehen.
func TestStyleOff(t *testing.T) {
	s := style{on: false}
	for _, got := range []string{s.bold("x"), s.dim("x"), s.accent("x"), s.warn("x"), s.ok("x")} {
		if got != "x" {
			t.Errorf("Stil ohne Farbe = %q, erwartet \"x\"", got)
		}
	}
}

// TestToTitleTurkish prüft die Groß-/Kleinschreibung von Ortsnamen.
func TestToTitleTurkish(t *testing.T) {
	cases := map[string]string{
		"WEINHEIM":          "Weinheim",
		"MANNHEIM":          "Mannheim",
		"BADEN WURTTEMBERG": "Baden Wurttemberg",
		"İSTANBUL":          "İstanbul",
	}
	for in, want := range cases {
		if got := toTitleTurkish(in); got != want {
			t.Errorf("toTitleTurkish(%q) = %q, erwartet %q", in, got, want)
		}
	}
}

// TestSearchMatches prüft die Standortsuche, insbesondere die deutschen
// ASCII-Umschreibungen. Diyanet schreibt deutsche Orte ohne Umlaute ("KOLN"),
// getippt wird auf einer deutschen Tastatur aber "koeln" oder "köln".
func TestSearchMatches(t *testing.T) {
	cases := []struct {
		haystack, needle string
		want             bool
	}{
		{"KOLN", "koeln", true},
		{"KOLN", "köln", true},
		{"KOLN", "koln", true},
		{"MUNCHEN", "muenchen", true},
		{"MUNCHEN", "münchen", true},
		{"DUSSELDORF", "duesseldorf", true},
		{"WEİNHEİM", "weinheim", true},
		{"WEİNHEİM", "wein", true},
		{"MANNHEIM", "koeln", false},
		{"NURNBERG", "nuernberg", true},
	}
	for _, c := range cases {
		if got := searchMatches(c.haystack, c.needle); got != c.want {
			t.Errorf("searchMatches(%q, %q) = %t, erwartet %t", c.haystack, c.needle, got, c.want)
		}
	}
}

// TestDeTranscribeGerman prüft die Auflösung einzeln.
func TestDeTranscribeGerman(t *testing.T) {
	cases := map[string]string{
		"koeln":        "koln",
		"muenchen":     "munchen",
		"saarbruecken": "saarbrucken",
		"strasse":      "strase",
		"weinheim":     "weinheim",
	}
	for in, want := range cases {
		if got := deTranscribeGerman(in); got != want {
			t.Errorf("deTranscribeGerman(%q) = %q, erwartet %q", in, got, want)
		}
	}
}
