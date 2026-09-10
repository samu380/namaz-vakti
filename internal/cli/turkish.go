package cli

import "strings"

// turkishFolds bildet türkische Sonderzeichen auf ASCII ab.
//
// Gebraucht wird das an zwei Stellen:
//
//  1. Kommandoauflösung — damit sowohl `namaz yarin` als auch `namaz yarın`
//     funktioniert.
//  2. Standortsuche — damit "weinheim" den API-Eintrag "WEİNHEİM" findet.
//
// Go's strings.ToLower kann das nicht leisten: es kennt die türkische
// i/ı-Unterscheidung nicht und macht aus "İ" ein "i̇" (i mit kombinierendem
// Punkt), was keinem ASCII-"i" gleicht.
var turkishFolds = map[rune]rune{
	'İ': 'i', 'I': 'i', 'ı': 'i', 'i': 'i',
	'Ş': 's', 'ş': 's',
	'Ğ': 'g', 'ğ': 'g',
	'Ü': 'u', 'ü': 'u',
	'Ö': 'o', 'ö': 'o',
	'Ç': 'c', 'ç': 'c',
	'Â': 'a', 'â': 'a',
	'Î': 'i', 'î': 'i',
	'Û': 'u', 'û': 'u',
}

// foldTurkish normalisiert einen String für Vergleiche: türkische Diakritika
// werden auf ASCII gefaltet, alles Übrige kleingeschrieben.
func foldTurkish(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if f, ok := turkishFolds[r]; ok {
			b.WriteRune(f)
			continue
		}
		b.WriteRune(toLowerASCII(r))
	}
	return b.String()
}

// toLowerASCII schreibt A-Z klein und lässt alles andere unverändert.
// Bewusst kein unicode.ToLower — das würde die oben behandelten türkischen
// Zeichen erneut anfassen.
func toLowerASCII(r rune) rune {
	if r >= 'A' && r <= 'Z' {
		return r + ('a' - 'A')
	}
	return r
}

// containsFolded meldet, ob needle in haystack vorkommt, beide gefaltet.
func containsFolded(haystack, needle string) bool {
	return strings.Contains(foldTurkish(haystack), foldTurkish(needle))
}

// germanTranscriptions bildet die deutschen ASCII-Umschreibungen ab.
//
// Diyanets Kreisliste schreibt deutsche Ortsnamen ohne Umlaute: "KOLN",
// "MUNCHEN", "DUSSELDORF". Getippt wird auf einer deutschen Tastatur aber
// gern "koeln" oder "muenchen". Ohne diese Auflösung findet die Suche nichts,
// obwohl der Ort existiert.
var germanTranscriptions = []struct{ from, to string }{
	{"oe", "o"},
	{"ue", "u"},
	{"ae", "a"},
	{"ss", "s"},
}

// deTranscribeGerman löst "oe" zu "o" auf und so weiter.
func deTranscribeGerman(s string) string {
	for _, t := range germanTranscriptions {
		s = strings.ReplaceAll(s, t.from, t.to)
	}
	return s
}

// searchMatches ist das Vergleichskriterium der Standortsuche.
//
// Getroffen wird, wenn der gefaltete Suchbegriff im gefalteten Namen
// vorkommt — oder wenn das nach Auflösen der deutschen Umschreibungen der
// Fall ist. Die zweite Runde erweitert nur die Treffermenge; da das Ergebnis
// als Liste angezeigt wird, sind gelegentliche Zusatztreffer unschädlich.
func searchMatches(haystack, needle string) bool {
	h, n := foldTurkish(haystack), foldTurkish(needle)
	if strings.Contains(h, n) {
		return true
	}
	return strings.Contains(deTranscribeGerman(h), deTranscribeGerman(n))
}
