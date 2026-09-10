// Package config lädt und schreibt die TOML-Konfiguration.
//
// Präzedenz der Werte: Flag > Umgebungsvariable > Config-Datei > Vorgabe.
// Damit `namaz ayar` anzeigen kann, woher ein Wert stammt, merkt sich Load
// zusätzlich, welche Schlüssel in der Datei tatsächlich gesetzt waren.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/samu380/namaz-vakti/pkg/vakit"
)

// AppName ist der Verzeichnisname unter ~/.config und ~/.cache.
const AppName = "namaz-vakti"

// Location ist ein in der Config hinterlegter Standort.
type Location struct {
	// DistrictID ist die Diyanet-İlçe-ID, z. B. "10532" für Weinheim.
	// Sie wird über `namaz konum ara` ermittelt.
	DistrictID string `toml:"ilce_id"`

	// Name ist der Anzeigename in der Kopfzeile.
	Name string `toml:"ad"`

	// TZ ist die IANA-Zeitzone, z. B. "Europe/Berlin". Sie ist zwingend:
	// die API liefert die Uhrzeiten ohne verlässliche Zonenangabe.
	TZ string `toml:"saat_dilimi"`
}

// LoadTZ lädt die Zeitzone des Standorts.
func (l Location) LoadTZ() (*time.Location, error) {
	if l.TZ == "" {
		return nil, errors.New("saat_dilimi tanımlı değil")
	}
	loc, err := time.LoadLocation(l.TZ)
	if err != nil {
		return nil, fmt.Errorf("saat dilimi %q yüklenemedi: %w", l.TZ, err)
	}
	return loc, nil
}

// KerahatConfig steuert Länge und Meldeverhalten der Kerahat-Fenster.
type KerahatConfig struct {
	// Minutenwerte der drei Fenster.
	Israk   int `toml:"israk"`
	Istiva  int `toml:"istiva"`
	Isfirar int `toml:"isfirar"`

	// Warn steuert, wann in der Tagesansicht gewarnt wird:
	//   "icerideyken" – erst wenn die Kerahat bereits läuft (Vorgabe)
	//   "erken"       – schon vorher, sobald sie ins laufende Fenster fällt
	//
	// Praktisch relevant ist das nur bei İkindi: die İsfirar-Kerahat ist das
	// einzige Fenster, das in ein Gebetsfenster hineinragt. İkindi bleibt
	// darin gültig, ist aber mekruh — die "weiche" Deadline liegt also 45
	// Minuten vor Akşam.
	Warn string `toml:"uyari"`
}

// Durations übersetzt die Minutenwerte ins Domänenmodell.
func (k KerahatConfig) Durations() vakit.KerahatDurations {
	return vakit.KerahatDurations{
		Israk:   time.Duration(k.Israk) * time.Minute,
		Istiva:  time.Duration(k.Istiva) * time.Minute,
		Isfirar: time.Duration(k.Isfirar) * time.Minute,
	}
}

// WarnEarly meldet, ob bereits vor Beginn der Kerahat gewarnt werden soll.
func (k KerahatConfig) WarnEarly() bool { return k.Warn == "erken" }

// Config ist der vollständige Konfigurationsbaum.
type Config struct {
	// DefaultLocation ist der Standort, der ohne -k verwendet wird.
	DefaultLocation string `toml:"varsayilan_konum"`

	// DefaultCountry schränkt `namaz konum ara` vorab ein, damit nicht die
	// Kreisliste der ganzen Welt geladen werden muss.
	DefaultCountry string `toml:"varsayilan_ulke"`

	// Provider wählt die Datenquelle (derzeit nur "ezanvakti").
	Provider string `toml:"saglayici"`

	// ShowQibla blendet die Kıble saati in der Tagesansicht ein.
	ShowQibla bool `toml:"kible_saati"`

	// Locations ist die Standorttabelle, Schlüssel ist der Kurzname ("ev").
	Locations map[string]Location `toml:"konumlar"`

	Kerahat KerahatConfig `toml:"kerahat"`

	// Offsets verschiebt einzelne Vakit-Zeiten um Minuten. Vorgesehen vor
	// allem für İkindi: Diyanet veröffentlicht die frühe Schattenlänge
	// (asr-ı evvel, Faktor 1). Wer den hanefitischen asr-ı sani möchte,
	// setzt hier je nach Jahreszeit rund +50.
	// Schlüssel sind die ASCII-Bezeichner aus vakit.Kind.Key().
	Offsets map[string]int `toml:"duzeltme"`

	// defined merkt sich, welche Schlüssel aus der Datei kamen. Nicht
	// serialisiert — nur für die Herkunftsspalte in `namaz ayar`.
	defined map[string]bool

	// path ist der Pfad, aus dem geladen wurde ("" wenn keine Datei existiert).
	path string
}

// Default liefert die Vorgabekonfiguration. Sie enthält bereits Weinheim als
// Standort "ev", damit das Tool nach der Installation sofort etwas anzeigt.
func Default() Config {
	return Config{
		DefaultLocation: "ev",
		DefaultCountry:  "ALMANYA",
		Provider:        "ezanvakti",
		ShowQibla:       false,
		Locations: map[string]Location{
			"ev": {DistrictID: "10532", Name: "Weinheim", TZ: "Europe/Berlin"},
		},
		Kerahat: KerahatConfig{Israk: 45, Istiva: 45, Isfirar: 45, Warn: "icerideyken"},
		Offsets: map[string]int{},
		defined: map[string]bool{},
	}
}

// DefaultPath liefert den Pfad der Config-Datei nach XDG-Konvention.
// Auf macOS wird bewusst ebenfalls ~/.config verwendet statt
// ~/Library/Application Support — das ist bei CLI-Werkzeugen üblich und hält
// die Konfiguration zwischen macOS und Linux austauschbar.
func DefaultPath() (string, error) {
	if p := os.Getenv("NAMAZ_CONFIG"); p != "" {
		return p, nil
	}
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("ev dizini bulunamadı: %w", err)
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, AppName, "config.toml"), nil
}

// CacheDir liefert das Cache-Verzeichnis nach XDG-Konvention.
func CacheDir() (string, error) {
	if p := os.Getenv("NAMAZ_CACHE"); p != "" {
		return p, nil
	}
	base := os.Getenv("XDG_CACHE_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("ev dizini bulunamadı: %w", err)
		}
		base = filepath.Join(home, ".cache")
	}
	return filepath.Join(base, AppName), nil
}

// Load liest die Config von path. Existiert die Datei nicht, wird die
// Vorgabekonfiguration zurückgegeben — das Tool ist also ohne vorherige
// Einrichtung lauffähig.
func Load(path string) (Config, error) {
	cfg := Default()
	cfg.path = path

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		cfg.path = ""
		return cfg, nil
	}
	if err != nil {
		return cfg, fmt.Errorf("%s okunamadı: %w", path, err)
	}

	// In die bereits mit Vorgaben gefüllte Struktur dekodieren: nicht
	// gesetzte Schlüssel behalten dadurch ihren Vorgabewert. Ausnahme sind
	// Maps — die ersetzt der Decoder komplett, was für konumlar genau richtig
	// ist (sonst ließe sich "ev" nie entfernen).
	md, err := toml.Decode(string(data), &cfg)
	if err != nil {
		return cfg, fmt.Errorf("%s çözümlenemedi: %w", path, err)
	}

	cfg.defined = map[string]bool{}
	for _, key := range md.Keys() {
		cfg.defined[strings.Join(key, ".")] = true
	}
	if cfg.Offsets == nil {
		cfg.Offsets = map[string]int{}
	}
	if cfg.Locations == nil {
		cfg.Locations = map[string]Location{}
	}

	return cfg, cfg.Validate()
}

// Validate prüft die Werte auf Plausibilität.
func (c Config) Validate() error {
	if c.Kerahat.Warn != "" && c.Kerahat.Warn != "erken" && c.Kerahat.Warn != "icerideyken" {
		return fmt.Errorf("kerahat.uyari %q geçersiz (erken | icerideyken)", c.Kerahat.Warn)
	}
	for _, m := range []struct {
		name string
		val  int
	}{{"israk", c.Kerahat.Israk}, {"istiva", c.Kerahat.Istiva}, {"isfirar", c.Kerahat.Isfirar}} {
		if m.val < 0 || m.val > 180 {
			return fmt.Errorf("kerahat.%s %d dakika geçersiz (0-180)", m.name, m.val)
		}
	}
	for key := range c.Offsets {
		if _, ok := vakit.ParseKind(key); !ok {
			return fmt.Errorf("duzeltme.%s bilinmeyen vakit", key)
		}
	}
	for name, l := range c.Locations {
		if l.DistrictID == "" {
			return fmt.Errorf("konumlar.%s: ilce_id eksik", name)
		}
		if _, err := l.LoadTZ(); err != nil {
			return fmt.Errorf("konumlar.%s: %w", name, err)
		}
	}
	return nil
}

// Path gibt den Pfad zurück, aus dem geladen wurde. Leer, wenn keine Datei
// existiert und ausschließlich Vorgaben aktiv sind.
func (c Config) Path() string { return c.path }

// WithPath setzt den Zielpfad, ohne die Datei zu lesen. Wird verwendet, wenn
// noch keine Config existiert, `namaz konum ekle` aber wissen muss, wohin
// geschrieben werden soll.
func (c Config) WithPath(path string) Config {
	c.path = path
	return c
}

// IsFromFile meldet, ob ein Schlüssel ("kerahat.israk") aus der Datei stammt.
func (c Config) IsFromFile(key string) bool { return c.defined[key] }

// Resolve löst einen Standortnamen auf. Ein leerer Name verwendet den
// Vorgabestandort.
func (c Config) Resolve(name string) (string, Location, error) {
	if name == "" {
		name = c.DefaultLocation
	}
	if len(c.Locations) == 0 {
		return "", Location{}, errors.New("hiç konum tanımlı değil — `namaz konum ara <ad>` ile ekleyin")
	}
	l, ok := c.Locations[name]
	if !ok {
		return "", Location{}, fmt.Errorf("konum %q bulunamadı (tanımlı: %s)", name, strings.Join(c.LocationNames(), ", "))
	}
	return name, l, nil
}

// LocationNames liefert die Standortnamen alphabetisch sortiert.
func (c Config) LocationNames() []string {
	names := make([]string, 0, len(c.Locations))
	for n := range c.Locations {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// OffsetDurations übersetzt die Minutenkorrekturen in ein Array, das direkt
// auf einen vakit.Day angewendet werden kann.
func (c Config) OffsetDurations() [vakit.KindCount]time.Duration {
	var out [vakit.KindCount]time.Duration
	for key, min := range c.Offsets {
		if k, ok := vakit.ParseKind(key); ok {
			out[k] = time.Duration(min) * time.Minute
		}
	}
	return out
}

// Save schreibt die Config nach path und legt fehlende Verzeichnisse an.
// Geschrieben wird über eine temporäre Datei, damit ein Absturz mitten im
// Schreiben die bestehende Konfiguration nicht zerstört.
func (c Config) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("dizin oluşturulamadı: %w", err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), ".config-*.toml")
	if err != nil {
		return fmt.Errorf("geçici dosya oluşturulamadı: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op, wenn das Rename geklappt hat

	enc := toml.NewEncoder(tmp)
	if err := enc.Encode(c); err != nil {
		tmp.Close()
		return fmt.Errorf("yapılandırma yazılamadı: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("geçici dosya kapatılamadı: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("%s dosyasına taşınamadı: %w", path, err)
	}
	return nil
}
