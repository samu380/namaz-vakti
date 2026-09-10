// Package cli enthält die Kommandozeilen-Oberfläche.
//
// Namenskonvention: Kommandos und Flags sind reines ASCII ("yarin", "surum",
// "--cevrimdisi"), damit sie auf einer deutschen Tastatur ohne Umwege
// tippbar sind. Die *Ausgabe* verwendet durchgehend korrekte türkische
// Schreibweise mit Diakritika ("İmsak", "Öğle", "Yatsı"). Kommandos mit
// Diakritika werden zusätzlich akzeptiert und intern gefaltet.
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/samu380/namaz-vakti/internal/cache"
	"github.com/samu380/namaz-vakti/internal/config"
	"github.com/samu380/namaz-vakti/internal/provider"
)

// Version wird beim Bauen über -ldflags gesetzt, siehe Makefile.
var Version = "dev"

// globals sind die Flags, die für jedes Kommando gelten. Sie dürfen sowohl
// vor als auch nach dem Kommando stehen (`namaz -k is sonraki` und
// `namaz sonraki -k is`), deshalb werden sie an beiden Stellen registriert.
type globals struct {
	Location   string // -k / --konum
	JSON       bool   // --json
	Color      string // --renk: oto | var | yok
	Refresh    bool   // --yenile
	Offline    bool   // --cevrimdisi
	ConfigPath string // --ayar
	Help       bool   // -y / --yardim
	ShowVer    bool   // --surum
}

// register hängt die globalen Flags an ein FlagSet.
//
// Als Vorgabewert dient jeweils der bereits geparste Wert. Dadurch überschreibt
// ein zweiter Parse-Durchlauf (nach dem Kommando) einen zuvor gesetzten Wert
// nicht mit der Nullvorgabe.
func (g *globals) register(fs *flag.FlagSet) {
	fs.StringVar(&g.Location, "konum", g.Location, "kullanılacak konum adı")
	fs.StringVar(&g.Location, "k", g.Location, "kısayol: --konum")
	fs.BoolVar(&g.JSON, "json", g.JSON, "makine okunabilir JSON çıktısı")
	fs.StringVar(&g.Color, "renk", g.Color, "renk kullanımı: oto | var | yok")
	fs.BoolVar(&g.Refresh, "yenile", g.Refresh, "önbelleği yok say, API'den yeniden yükle")
	fs.BoolVar(&g.Offline, "cevrimdisi", g.Offline, "ağa hiç çıkma, yalnızca önbellek")
	fs.StringVar(&g.ConfigPath, "ayar", g.ConfigPath, "alternatif yapılandırma dosyası")
	fs.BoolVar(&g.Help, "yardim", g.Help, "yardımı göster")
	fs.BoolVar(&g.Help, "y", g.Help, "kısayol: --yardim")
	fs.BoolVar(&g.Help, "help", g.Help, "yardımı göster")
	fs.BoolVar(&g.Help, "h", g.Help, "kısayol: --help")
	fs.BoolVar(&g.ShowVer, "surum", g.ShowVer, "sürümü göster")
	fs.BoolVar(&g.ShowVer, "version", g.ShowVer, "sürümü göster")
}

// env bündelt alles, was die Kommandos zur Ausführung brauchen.
type env struct {
	g      *globals
	cfg    config.Config
	cache  *cache.Cache
	style  style
	stdout io.Writer
	stderr io.Writer

	// now ist der Bezugszeitpunkt. Als Feld statt time.Now(), damit Tests
	// jeden Tageszeitpunkt reproduzierbar durchspielen können.
	now time.Time
}

// Run ist der Einstiegspunkt. Der Rückgabewert ist der Exit-Code.
func Run(args []string, stdout, stderr io.Writer) int {
	g := &globals{Color: "oto"}

	// Erster Durchlauf: Flags vor dem Kommando einsammeln. flag.Parse hält
	// beim ersten Nicht-Flag-Argument an, das ist genau das Kommando.
	root := flag.NewFlagSet("namaz", flag.ContinueOnError)
	root.SetOutput(io.Discard) // eigene Fehlermeldungen statt der Standardausgabe
	g.register(root)
	if err := root.Parse(args); err != nil {
		fmt.Fprintf(stderr, "hata: %v\n\n", err)
		printUsage(stderr)
		return 2
	}

	rest := root.Args()
	cmd := "gun" // Vorgabe: Tagesansicht für heute
	if len(rest) > 0 {
		cmd = foldTurkish(rest[0])
		rest = rest[1:]
	}

	if g.ShowVer {
		fmt.Fprintf(stdout, "namaz %s\n", Version)
		return 0
	}
	// `namaz --yardim` ohne Kommando zeigt die Gesamtübersicht.
	if g.Help && len(rest) == 0 && cmd == "gun" {
		printUsage(stdout)
		return 0
	}

	if err := dispatch(cmd, rest, g, stdout, stderr); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintf(stderr, "hata: %v\n", err)
		return 1
	}
	return 0
}

// dispatch wählt das Kommando aus und baut die Ausführungsumgebung auf.
func dispatch(cmd string, args []string, g *globals, stdout, stderr io.Writer) error {
	switch cmd {
	case "yardim", "help":
		printUsage(stdout)
		return nil
	case "surum", "version":
		fmt.Fprintf(stdout, "namaz %s\n", Version)
		return nil
	}

	// Ab hier brauchen alle Kommandos Config und Cache.
	e, err := newEnv(g, stdout, stderr)
	if err != nil {
		return err
	}

	switch cmd {
	case "gun", "bugun":
		return cmdDay(e, args, dayToday)
	case "yarin":
		return cmdDay(e, args, dayTomorrow)
	case "tarih":
		return cmdDay(e, args, dayExplicit)
	case "sonraki":
		return cmdNext(e, args)
	case "kerahat":
		return cmdKerahat(e, args)
	case "konum":
		return cmdLocation(e, args)
	case "ayar":
		return cmdSettings(e, args)
	default:
		printUsage(stderr)
		return fmt.Errorf("bilinmeyen komut %q", cmd)
	}
}

// newEnv lädt Config und Cache und richtet die Farbausgabe ein.
func newEnv(g *globals, stdout, stderr io.Writer) (*env, error) {
	path := g.ConfigPath
	if path == "" {
		p, err := config.DefaultPath()
		if err != nil {
			return nil, err
		}
		path = p
	}

	cfg, err := config.Load(path)
	if err != nil {
		return nil, err
	}
	// Beim Laden über --ayar merken wir uns den Pfad auch dann, wenn die
	// Datei (noch) nicht existiert — `konum ekle` schreibt dann dorthin.
	if cfg.Path() == "" {
		cfg = cfg.WithPath(path)
	}

	p, err := provider.Get(cfg.Provider)
	if err != nil {
		return nil, err
	}

	dir, err := config.CacheDir()
	if err != nil {
		return nil, err
	}
	c := cache.New(dir, p)
	c.Offline = g.Offline
	c.Refresh = g.Refresh

	return &env{
		g:      g,
		cfg:    cfg,
		cache:  c,
		style:  newStyle(g.Color, stdout),
		stdout: stdout,
		stderr: stderr,
		now:    time.Now(),
	}, nil
}

// ctx liefert einen Kontext mit Zeitlimit für Netzwerkaufrufe.
func (e *env) ctx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 30*time.Second)
}

// subFlags erzeugt ein FlagSet für ein Unterkommando, an dem die globalen
// Flags erneut hängen.
func (e *env) subFlags(name string) *flag.FlagSet {
	fs := flag.NewFlagSet("namaz "+name, flag.ContinueOnError)
	fs.SetOutput(e.stderr)
	e.g.register(fs)
	return fs
}

// parse wertet die Argumente eines Unterkommandos aus und gibt die
// Positionsargumente zurück.
//
// Nötig, weil flag.Parse beim ersten Nicht-Flag-Argument abbricht: bei
// `namaz konum ekle ev --ilce 10532` würde --ilce sonst nie ausgewertet.
// Deshalb werden führende Positionsargumente vorab abgetrennt und danach
// wieder mit den verbliebenen zusammengeführt. Damit funktionieren beide
// Schreibweisen — Flags vor wie hinter den Argumenten.
func (e *env) parse(fs *flag.FlagSet, args []string) ([]string, error) {
	lead, rest := splitLeadingArgs(args)
	if err := fs.Parse(rest); err != nil {
		return nil, err
	}
	return append(lead, fs.Args()...), nil
}

// splitLeadingArgs trennt die führenden Positionsargumente von allem, was ab
// dem ersten Flag folgt.
//
// Ein einzelnes "-" gilt als Positionsargument (üblicherweise "Standardeingabe"),
// "--" beendet die Flag-Erkennung und wird an flag.Parse weitergegeben.
func splitLeadingArgs(args []string) (positional, flags []string) {
	for i, a := range args {
		if len(a) > 1 && a[0] == '-' {
			return args[:i:i], args[i:]
		}
	}
	return args, nil
}

const usage = `namaz — Diyanet namaz vakitleri

KULLANIM
  namaz [komut] [seçenekler]

KOMUTLAR
  (yok)               bugünün vakitleri, sıradaki vakte kalan süre
  bugun               aynısı, açıkça
  yarin               yarının vakitleri
  tarih <YYYY-AA-GG>  belirli bir günün vakitleri
  sonraki             tek satır: sıradaki vakit ve kalan süre
  kerahat             günün üç kerahat vakti
  konum               konumları yönet (liste | ara | ekle | sil | varsayilan)
  ayar                yapılandırmayı göster (goster | yol)
  surum               sürüm bilgisi
  yardim              bu yardım

GENEL SEÇENEKLER
  -k, --konum <ad>    kullanılacak konum (varsayılan: yapılandırmadan)
      --json          JSON çıktısı
      --renk <mod>    oto | var | yok   (varsayılan: oto)
      --yenile        önbelleği yok say, API'den yeniden yükle
      --cevrimdisi    ağa çıkma, yalnızca önbellek
      --ayar <yol>    alternatif yapılandırma dosyası
  -y, --yardim        yardım
      --surum         sürüm

ÖRNEKLER
  namaz                        bugünün vakitleri
  namaz -k is                  iş konumu için
  namaz yarin                  yarının vakitleri
  namaz tarih 2026-12-24
  namaz sonraki --bicim "{vakit} {kalan}"
  namaz konum ara weinheim
  namaz konum ekle is --ilce 11021 --sd Europe/Berlin
  namaz --json | jq .sonraki

Komutlar Türkçe diakritiksiz yazılır (yarin, surum). Diakritikli yazım da
kabul edilir. Yapılandırma: ~/.config/namaz-vakti/config.toml
`

func printUsage(w io.Writer) { fmt.Fprint(w, usage) }

// Main ist der von cmd/namaz aufgerufene Einstiegspunkt.
func Main() {
	os.Exit(Run(os.Args[1:], os.Stdout, os.Stderr))
}
