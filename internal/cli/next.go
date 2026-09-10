package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/samu380/namaz-vakti/pkg/vakit"
)

// cmdNext bedient `namaz sonraki` — eine einzige Zeile, sonst nichts.
//
// Das ist die Schnittstelle für Statusleisten: SwiftBar, tmux, Waybar oder
// polybar rufen dieses Kommando im Minutentakt auf. Weil der Kalender lokal
// gecacht ist, kostet ein Aufruf keinen Netzwerkzugriff.
func cmdNext(e *env, args []string) error {
	fs := e.subFlags("sonraki")
	format := fs.String("bicim", "", "çıktı şablonu, örn: \"{vakit} {kalan}\"")
	short := fs.Bool("kisa", false, "kısa biçim: \"İkindi 2:54\"")
	withSec := fs.Bool("saniye", false, "geri sayımı saniye duyarlığında göster")
	if _, err := e.parse(fs, args); err != nil {
		return err
	}
	if e.g.Help {
		fs.Usage()
		fmt.Fprint(e.stderr, nextFormatHelp)
		return nil
	}

	r, err := e.resolveLocation()
	if err != nil {
		return err
	}
	now := e.now.In(r.TZ)

	cal, meta, err := e.loadCalendar(r, now)
	if err != nil {
		return err
	}
	st, err := cal.StatusAt(now, e.cfg.Kerahat.Durations())
	if err != nil {
		return err
	}
	if st.Next == nil {
		return fmt.Errorf("sıradaki vakit belirlenemedi (takvim eksik olabilir)")
	}
	until, _ := st.UntilNext()

	if e.g.JSON {
		out := buildStatusJSON(r, st, e.cfg.Kerahat.Durations(), jsonSource{
			Provider:  e.cfg.Provider,
			FromCache: meta.FromCache,
			FetchedAt: iso(meta.FetchedAt),
		})
		// Für den Einzeiler interessieren nur Standort, nächstes Vakit und
		// Kerahat — die Tagestabelle würde die Ausgabe unnötig aufblähen.
		out.Times = nil
		return writeJSONOut(e.stdout, out)
	}

	switch {
	case *format != "":
		fmt.Fprintln(e.stdout, expandFormat(*format, r, st, until, *withSec))
	case *short:
		fmt.Fprintf(e.stdout, "%s %s\n", st.Next.Prayer, formatDurationShort(until))
	default:
		fmt.Fprintf(e.stdout, "%s %s · %s\n",
			st.Next.Prayer, hhmm(st.Next.At), FormatDuration(until, *withSec))
	}
	return nil
}

const nextFormatHelp = `
--bicim yer tutucuları:
  {vakit}     sıradaki vaktin adı        İkindi
  {saat}      sıradaki vaktin saati      17:06
  {kalan}     kalan süre                 2 sa 54 dk
  {kalan_ks}  kalan süre, kısa           2:54
  {kalan_dk}  kalan süre, dakika olarak  174
  {kalan_sn}  kalan süre, saniye olarak  10440
  {konum}     konumun adı                Weinheim
  {kerahat}   etkin kerahat, yoksa boş   İsfirar

Örnek:
  namaz sonraki --bicim "{vakit} {kalan_ks}"
`

// expandFormat ersetzt die Platzhalter in einer --bicim-Vorlage.
//
// Bewusst kein text/template: für eine Statusleisten-Vorlage wäre dessen
// Syntax ({{.Foo}}) umständlicher zu tippen und die Fehlermeldungen wären
// für diesen Zweck unverhältnismäßig.
func expandFormat(format string, r resolved, st vakit.Status, until time.Duration, withSec bool) string {
	kerahat := ""
	if st.Kerahat != nil {
		kerahat = st.Kerahat.Kind.Label()
	}
	name := r.Loc.Name
	if name == "" {
		name = r.Key
	}

	repl := strings.NewReplacer(
		"{vakit}", st.Next.Prayer,
		"{saat}", hhmm(st.Next.At),
		"{kalan}", FormatDuration(until, withSec),
		"{kalan_ks}", formatDurationShort(until),
		"{kalan_dk}", fmt.Sprintf("%d", int(until.Minutes())),
		"{kalan_sn}", fmt.Sprintf("%d", int(until.Seconds())),
		"{konum}", name,
		"{kerahat}", kerahat,
	)
	return repl.Replace(format)
}
