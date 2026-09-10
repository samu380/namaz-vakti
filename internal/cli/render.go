package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/samu380/namaz-vakti/pkg/vakit"
)

// contentWidth ist die Breite des Textblocks, an der die Kopfzeile rechts
// ausgerichtet wird. 54 Zeichen plus 2 Zeichen Einzug passen in jedes
// vernünftig breite Terminal.
const contentWidth = 54

// --- Farben -----------------------------------------------------------------

// ANSI-Sequenzen. Bewusst nur wenige und keine Hintergrundfarben: die Ausgabe
// soll auf hellen wie dunklen Terminal-Themes lesbar bleiben.
const (
	ansiReset  = "\033[0m"
	ansiBold   = "\033[1m"
	ansiDim    = "\033[2m"
	ansiCyan   = "\033[36m"
	ansiYellow = "\033[33m"
	ansiGreen  = "\033[32m"
)

// style kapselt die Farbausgabe. Ist sie deaktiviert, geben alle Methoden den
// Text unverändert zurück — die Aufrufer brauchen keine Fallunterscheidung.
type style struct{ on bool }

// newStyle entscheidet anhand von --renk und der Umgebung über Farbe.
//
// Bei "oto" gilt: Farbe nur, wenn die Ausgabe wirklich in ein Terminal geht.
// Damit bleibt `namaz | grep` und `namaz > datei.txt` frei von Steuerzeichen.
// NO_COLOR wird respektiert (siehe no-color.org).
func newStyle(mode string, w io.Writer) style {
	switch mode {
	case "var", "always":
		return style{on: true}
	case "yok", "never":
		return style{on: false}
	}
	if os.Getenv("NO_COLOR") != "" {
		return style{on: false}
	}
	if os.Getenv("TERM") == "dumb" {
		return style{on: false}
	}
	f, ok := w.(*os.File)
	if !ok {
		return style{on: false}
	}
	info, err := f.Stat()
	if err != nil {
		return style{on: false}
	}
	return style{on: info.Mode()&os.ModeCharDevice != 0}
}

func (s style) wrap(code, text string) string {
	if !s.on || text == "" {
		return text
	}
	return code + text + ansiReset
}

func (s style) bold(t string) string   { return s.wrap(ansiBold, t) }
func (s style) dim(t string) string    { return s.wrap(ansiDim, t) }
func (s style) accent(t string) string { return s.wrap(ansiCyan, t) }
func (s style) warn(t string) string   { return s.wrap(ansiYellow, t) }
func (s style) ok(t string) string     { return s.wrap(ansiGreen, t) }

// --- Ausrichtung ------------------------------------------------------------

// runeLen zählt Zeichen, nicht Bytes. "İmsak" belegt 6 Bytes, aber 5 Spalten.
func runeLen(s string) int { return utf8.RuneCountInString(s) }

// padRight füllt s rechts mit Leerzeichen auf n Zeichen auf.
func padRight(s string, n int) string {
	if d := n - runeLen(s); d > 0 {
		return s + strings.Repeat(" ", d)
	}
	return s
}

// padLeft füllt s links mit Leerzeichen auf n Zeichen auf.
func padLeft(s string, n int) string {
	if d := n - runeLen(s); d > 0 {
		return strings.Repeat(" ", d) + s
	}
	return s
}

// --- Dauer ------------------------------------------------------------------

// FormatDuration schreibt eine Dauer in türkischer Kurzform.
//
//	2h54m  → "2 sa 54 dk"
//	54m    → "54 dk"
//	40s    → "1 dk'dan az"  (bzw. "40 sn" mit seconds=true)
//
// Negative Werte werden auf null geklemmt: sie entstehen nur durch
// Rundungsränder und "-0 dk" wäre verwirrend.
func FormatDuration(d time.Duration, seconds bool) string {
	if d < 0 {
		d = 0
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60

	switch {
	case h > 0 && seconds:
		return fmt.Sprintf("%d sa %d dk %d sn", h, m, s)
	case h > 0:
		return fmt.Sprintf("%d sa %d dk", h, m)
	case m > 0 && seconds:
		return fmt.Sprintf("%d dk %d sn", m, s)
	case m > 0:
		return fmt.Sprintf("%d dk", m)
	case seconds:
		return fmt.Sprintf("%d sn", s)
	default:
		return "1 dk'dan az"
	}
}

// formatDurationShort schreibt eine Dauer als "s:mm" — kompakt genug für
// Statusleisten.
func formatDurationShort(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	return fmt.Sprintf("%d:%02d", int(d.Hours()), int(d.Minutes())%60)
}

// hhmm formatiert eine Uhrzeit. Leere Zeiten werden zu "—", damit die Spalte
// nicht zusammenbricht, wenn die API ein Feld nicht geliefert hat.
func hhmm(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.Format("15:04")
}

// --- Tagesansicht -----------------------------------------------------------

// renderHeader schreibt Standort links, Datum und Hicri-Datum rechts.
//
// key ist der Config-Schlüssel ("ev"), name der Anzeigename ("Weinheim").
// Angezeigt wird "Weinheim · ev", damit bei mehreren Standorten auf einen
// Blick klar ist, welcher gerade gilt.
func (e *env) renderHeader(w io.Writer, key, name string, day vakit.Day) {
	left := name
	if key != "" {
		left = name + " · " + key
	}
	right := day.FormatDate()

	// Kollidieren beide Seiten, kommt das Datum auf eine eigene Zeile.
	if runeLen(left)+runeLen(right)+2 > contentWidth {
		fmt.Fprintf(w, "  %s\n", e.style.bold(left))
		fmt.Fprintf(w, "  %s\n", e.style.dim(right))
	} else {
		fmt.Fprintf(w, "  %s%s\n",
			e.style.bold(padRight(left, contentWidth-runeLen(right))),
			e.style.dim(right))
	}
	if day.HijriLong != "" {
		fmt.Fprintf(w, "  %s\n", e.style.dim(padLeft(day.HijriLong, contentWidth)))
	}
}

// renderTimes schreibt die sechs Zeitmarken. Ist ein Gebetsfenster aktiv,
// wird dessen Startmarke hervorgehoben.
func (e *env) renderTimes(w io.Writer, st vakit.Status) {
	var currentKind vakit.Kind = -1
	if st.Current != nil {
		currentKind = st.Current.Opens
	}

	for _, k := range vakit.AllKinds {
		label := padRight(k.Label(), 9)
		t := hhmm(st.Today.At(k))

		if k == currentKind {
			fmt.Fprintf(w, "    %s%s  %s\n",
				e.style.bold(label), e.style.bold(t), e.style.accent("▸ şu an"))
			continue
		}
		// Güneş ist kein Gebet — dezenter darstellen, damit die fünf
		// Gebetszeiten optisch führen.
		if k == vakit.Gunes {
			fmt.Fprintf(w, "    %s%s\n", e.style.dim(label), e.style.dim(t))
			continue
		}
		fmt.Fprintf(w, "    %s%s\n", label, t)
	}
}

// statusLines baut die Countdown-Zeilen.
//
// Die Regel: Die Deadline steht immer oben. Endet das laufende Fenster genau
// dann, wenn das nächste Gebet beginnt — der Normalfall bei Öğle→İkindi,
// İkindi→Akşam, Akşam→Yatsı und Yatsı→Sabah —, sagt eine einzige Zeile beides.
// Nur im Sabah-Fenster fallen die Zahlen auseinander (es endet mit Güneş,
// während Öğle erst Stunden später beginnt), dann werden es zwei Zeilen.
// In der Vormittagslücke zwischen Güneş und Öğle ist gar kein Fenster aktiv.
func (e *env) statusLines(st vakit.Status) []string {
	var out []string

	switch {
	case st.Current != nil && st.NextIsWindowEnd():
		rem, _ := st.Remaining()
		out = append(out, fmt.Sprintf("%s vaktinin bitişine %s — %s başlıyor (%s)",
			st.Current.Prayer,
			e.style.bold(FormatDuration(rem, false)),
			st.Next.Prayer,
			hhmm(st.Next.At)))

	case st.Current != nil:
		rem, _ := st.Remaining()
		out = append(out, fmt.Sprintf("%s vaktinin bitişine %s (%s)",
			st.Current.Prayer,
			e.style.bold(FormatDuration(rem, false)),
			hhmm(st.Current.To)))
		if st.Next != nil {
			until, _ := st.UntilNext()
			out = append(out, e.style.dim(fmt.Sprintf("Sonraki vakit: %s %s · %s",
				st.Next.Prayer, hhmm(st.Next.At), FormatDuration(until, false))))
		}

	default:
		out = append(out, e.style.dim("Şu an farz namaz vakti değil"))
		if st.Next != nil {
			until, _ := st.UntilNext()
			out = append(out, fmt.Sprintf("Sonraki vakit: %s %s · %s",
				st.Next.Prayer, hhmm(st.Next.At), e.style.bold(FormatDuration(until, false))))
		}
	}
	return out
}

// kerahatLine meldet eine laufende Kerahat — oder, wenn kerahat.uyari auf
// "erken" steht, auch eine, die im laufenden Gebetsfenster noch bevorsteht.
//
// Praktisch betrifft die Vorwarnung nur İkindi: die İsfirar-Kerahat ist das
// einzige Fenster, das in ein Gebetsfenster hineinragt. İkindi bleibt darin
// gültig, ist aber mekruh — die "weiche" Deadline liegt also 45 Minuten vor
// Akşam. Vorgabe ist "icerideyken", also die zurückhaltendere Variante.
func (e *env) kerahatLine(st vakit.Status) string {
	if st.Kerahat != nil {
		rem, _ := st.KerahatRemaining()
		return e.style.warn(fmt.Sprintf("⚠ Kerahat vakti (%s) — bitişine %s",
			st.Kerahat.Kind.Label(), FormatDuration(rem, false)))
	}

	if !e.cfg.Kerahat.WarnEarly() || st.Current == nil {
		return ""
	}
	for _, k := range st.Today.Kerahats(e.cfg.Kerahat.Durations()) {
		// Nur Kerahat-Fenster, die noch im laufenden Gebetsfenster beginnen.
		if k.Start.After(st.Now) && k.Start.Before(st.Current.To) {
			return e.style.warn(fmt.Sprintf("⚠ %s kerahatinin başlangıcına %s (%s)",
				k.Kind.Label(), FormatDuration(k.Start.Sub(st.Now), false), hhmm(k.Start)))
		}
	}
	return ""
}

// renderStatusDay schreibt die vollständige Tagesansicht für heute,
// einschließlich Countdown und Kerahat-Hinweis.
func (e *env) renderStatusDay(w io.Writer, key, name string, st vakit.Status) {
	fmt.Fprintln(w)
	e.renderHeader(w, key, name, st.Today)
	fmt.Fprintln(w)
	e.renderTimes(w, st)
	fmt.Fprintln(w)

	for _, line := range e.statusLines(st) {
		fmt.Fprintf(w, "  %s\n", line)
	}
	if k := e.kerahatLine(st); k != "" {
		fmt.Fprintf(w, "  %s\n", k)
	}
	if e.cfg.ShowQibla && !st.Today.QiblaTime.IsZero() {
		fmt.Fprintf(w, "  %s\n", e.style.dim("Kıble saati "+hhmm(st.Today.QiblaTime)))
	}
	fmt.Fprintln(w)
}

// renderPlainDay schreibt die Tabelle ohne Countdown — für andere Tage als
// heute, wo eine Restzeit keinen Sinn ergäbe.
func (e *env) renderPlainDay(w io.Writer, key, name string, day vakit.Day) {
	fmt.Fprintln(w)
	e.renderHeader(w, key, name, day)
	fmt.Fprintln(w)
	for _, k := range vakit.AllKinds {
		label := padRight(k.Label(), 9)
		if k == vakit.Gunes {
			fmt.Fprintf(w, "    %s%s\n", e.style.dim(label), e.style.dim(hhmm(day.At(k))))
			continue
		}
		fmt.Fprintf(w, "    %s%s\n", label, hhmm(day.At(k)))
	}
	if e.cfg.ShowQibla && !day.QiblaTime.IsZero() {
		fmt.Fprintf(w, "\n  %s\n", e.style.dim("Kıble saati "+hhmm(day.QiblaTime)))
	}
	fmt.Fprintln(w)
}
