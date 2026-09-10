package cli

import (
	"fmt"
	"time"

	"github.com/samu380/namaz-vakti/pkg/vakit"
)

// cmdSettings bedient `namaz ayar` (Übersicht) und `namaz ayar yol` (Pfad).
func cmdSettings(e *env, args []string) error {
	sub := "goster"
	if len(args) > 0 && args[0][0] != '-' {
		sub = foldTurkish(args[0])
		args = args[1:]
	}

	fs := e.subFlags("ayar " + sub)
	if _, err := e.parse(fs, args); err != nil {
		return err
	}

	switch sub {
	case "yol":
		// Nur der Pfad, ohne Dekoration — so lässt er sich direkt
		// weiterverwenden: $EDITOR "$(namaz ayar yol)"
		//
		// Der Pfad ist auch dann gesetzt, wenn die Datei noch nicht
		// existiert; newEnv trägt ihn ein, damit `konum ekle` weiß, wohin
		// geschrieben werden soll.
		fmt.Fprintln(e.stdout, e.cfg.Path())
		return nil

	case "goster":
		return settingsShow(e)

	default:
		return fmt.Errorf("bilinmeyen alt komut %q (goster | yol)", sub)
	}
}

// settingRow ist eine Zeile der Übersicht: Schlüssel, Wert, Herkunft.
type settingRow struct {
	Key    string
	Value  string
	Source string
}

// settingsShow zeigt die effektive Konfiguration mit Herkunftsspalte.
//
// Die Herkunft ist der eigentliche Zweck des Kommandos: bei vier
// Präzedenzebenen (Flag > Umgebung > Datei > Vorgabe) ist sonst nicht
// erkennbar, warum ein Wert so ist, wie er ist — etwa ob ein gesetzter
// İkindi-Offset überhaupt greift.
func settingsShow(e *env) error {
	origin := func(key string) string {
		if e.cfg.IsFromFile(key) {
			return "yapılandırma"
		}
		return "varsayılan"
	}

	rows := []settingRow{
		{"varsayilan_konum", e.cfg.DefaultLocation, origin("varsayilan_konum")},
		{"varsayilan_ulke", e.cfg.DefaultCountry, origin("varsayilan_ulke")},
		{"saglayici", e.cfg.Provider, origin("saglayici")},
		{"kible_saati", fmt.Sprintf("%t", e.cfg.ShowQibla), origin("kible_saati")},
		{"kerahat.israk", fmt.Sprintf("%d dk", e.cfg.Kerahat.Israk), origin("kerahat.israk")},
		{"kerahat.istiva", fmt.Sprintf("%d dk", e.cfg.Kerahat.Istiva), origin("kerahat.istiva")},
		{"kerahat.isfirar", fmt.Sprintf("%d dk", e.cfg.Kerahat.Isfirar), origin("kerahat.isfirar")},
		{"kerahat.uyari", e.cfg.Kerahat.Warn, origin("kerahat.uyari")},
	}

	// Korrekturen: alle sechs Vakit-Zeiten zeigen, damit sichtbar ist, dass
	// es sie gibt — auch wenn sie auf 0 stehen.
	for _, k := range vakit.AllKinds {
		key := "duzeltme." + k.Key()
		rows = append(rows, settingRow{
			key,
			fmt.Sprintf("%+d dk", e.cfg.Offsets[k.Key()]),
			origin(key),
		})
	}

	if e.g.JSON {
		type row struct {
			Key    string `json:"anahtar"`
			Value  string `json:"deger"`
			Source string `json:"kaynak"`
		}
		out := make([]row, 0, len(rows))
		for _, r := range rows {
			out = append(out, row{r.Key, r.Value, r.Source})
		}
		return writeJSONOut(e.stdout, out)
	}

	fmt.Fprintf(e.stdout, "\n  %s %s\n\n", e.style.dim("Yapılandırma:"), e.cfg.Path())

	for _, r := range rows {
		fmt.Fprintf(e.stdout, "    %s%s %s\n",
			padRight(r.Key, 22),
			padRight(r.Value, 16),
			e.style.dim("("+r.Source+")"))
	}

	// Cache-Zeile: ersetzt ein eigenes `namaz onbellek`-Kommando. Sichtbar
	// wird damit, ob die angezeigten Zeiten frisch sind und welchen Zeitraum
	// der Cache abdeckt.
	fmt.Fprintf(e.stdout, "\n  %s\n", e.style.dim("Önbellek: "+e.cache.Dir()))
	for _, name := range e.cfg.LocationNames() {
		loc := e.cfg.Locations[name]
		meta, ok := e.cache.CalendarInfo(loc.DistrictID)
		if !ok {
			fmt.Fprintf(e.stdout, "    %s%s\n", padRight(name, 12), e.style.dim("yok"))
			continue
		}
		fmt.Fprintf(e.stdout, "    %s%s – %s   %s\n",
			padRight(name, 12),
			meta.First.Format(inputDateLayout),
			meta.Last.Format(inputDateLayout),
			e.style.dim("güncellendi: "+humanAge(meta.FetchedAt, e.now)))
	}
	fmt.Fprintln(e.stdout)
	return nil
}

// humanAge beschreibt, wie alt ein Zeitstempel ist.
func humanAge(t, now time.Time) string {
	if t.IsZero() {
		return "bilinmiyor"
	}
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "az önce"
	case d < time.Hour:
		return fmt.Sprintf("%d dk önce", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%d sa önce", int(d.Hours()))
	default:
		return fmt.Sprintf("%d gün önce", int(d.Hours()/24))
	}
}
