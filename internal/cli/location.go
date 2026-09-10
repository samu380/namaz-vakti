package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/samu380/namaz-vakti/internal/config"
	"github.com/samu380/namaz-vakti/internal/provider"
)

// cmdLocation bedient `namaz konum` mit seinen Unterkommandos.
func cmdLocation(e *env, args []string) error {
	sub := "liste"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		sub = foldTurkish(args[0])
		args = args[1:]
	}

	switch sub {
	case "liste":
		return locationList(e, args)
	case "ara":
		return locationSearch(e, args)
	case "ekle":
		return locationAdd(e, args)
	case "sil":
		return locationRemove(e, args)
	case "varsayilan":
		return locationDefault(e, args)
	default:
		return fmt.Errorf("bilinmeyen alt komut %q (liste | ara | ekle | sil | varsayilan)", sub)
	}
}

// --- liste ------------------------------------------------------------------

func locationList(e *env, args []string) error {
	fs := e.subFlags("konum liste")
	if _, err := e.parse(fs, args); err != nil {
		return err
	}

	names := e.cfg.LocationNames()
	if len(names) == 0 {
		fmt.Fprintln(e.stdout, "Hiç konum tanımlı değil. `namaz konum ara <ad>` ile arayın.")
		return nil
	}

	if e.g.JSON {
		type row struct {
			Key        string `json:"anahtar"`
			Name       string `json:"ad"`
			DistrictID string `json:"ilce_id"`
			TZ         string `json:"saat_dilimi"`
			Default    bool   `json:"varsayilan"`
		}
		out := make([]row, 0, len(names))
		for _, n := range names {
			l := e.cfg.Locations[n]
			out = append(out, row{n, l.Name, l.DistrictID, l.TZ, n == e.cfg.DefaultLocation})
		}
		return writeJSONOut(e.stdout, out)
	}

	fmt.Fprintln(e.stdout)
	for _, n := range names {
		l := e.cfg.Locations[n]
		marker := " "
		if n == e.cfg.DefaultLocation {
			marker = "*"
		}
		name := l.Name
		if name == "" {
			name = "—"
		}
		fmt.Fprintf(e.stdout, "    %s %s %s %s  %s\n",
			e.style.accent(marker),
			padRight(n, 10),
			padRight(name, 20),
			padRight(l.DistrictID, 8),
			e.style.dim(l.TZ))
	}
	fmt.Fprintf(e.stdout, "\n  %s\n\n", e.style.dim("* = varsayılan konum"))
	return nil
}

// --- ara --------------------------------------------------------------------

func locationSearch(e *env, args []string) error {
	fs := e.subFlags("konum ara")
	country := fs.String("ulke", e.cfg.DefaultCountry, "aranacak ülke adı veya ID")
	rest, err := e.parse(fs, args)
	if err != nil {
		return err
	}

	needle := strings.TrimSpace(strings.Join(rest, " "))
	if needle == "" {
		return fmt.Errorf("aranacak metin gerekli, örn: namaz konum ara weinheim")
	}

	ctx, cancel := e.ctx()
	defer cancel()

	countries, err := e.cache.Countries(ctx)
	if err != nil {
		return err
	}
	c, ok := pickPlace(countries, *country)
	if !ok {
		return fmt.Errorf("ülke %q bulunamadı — mevcut ülkeleri görmek için: namaz konum ara --ulke <ad> <metin>", *country)
	}

	states, err := e.cache.States(ctx, c.ID)
	if err != nil {
		return err
	}

	// Die Kreislisten werden pro Bundesland geholt. Beim ersten Lauf sind
	// das für Deutschland 16 Anfragen; danach liegt alles im Cache und die
	// Suche ist rein lokal. Der Hinweis geht nach stderr, damit `--json`
	// und Pipes davon unberührt bleiben.
	if !e.allStatesCached(states) {
		fmt.Fprintf(e.stderr, "%s ilçe listeleri indiriliyor (ilk kez, sonra önbellekten)...\n", c.Name)
	}

	type hit struct {
		State    provider.Place
		District provider.Place
	}
	var hits []hit

	for _, s := range states {
		districts, err := e.cache.Districts(ctx, s.ID)
		if err != nil {
			fmt.Fprintf(e.stderr, "uyarı: %s ilçeleri alınamadı: %v\n", s.Name, err)
			continue
		}
		for _, d := range districts {
			if searchMatches(d.Name, needle) || searchMatches(d.NameEn, needle) {
				hits = append(hits, hit{State: s, District: d})
			}
		}
	}

	if len(hits) == 0 {
		return fmt.Errorf("%s içinde %q ile eşleşen ilçe bulunamadı", c.Name, needle)
	}

	if e.g.JSON {
		type row struct {
			CountryID string `json:"ulke_id"`
			Country   string `json:"ulke"`
			StateID   string `json:"sehir_id"`
			State     string `json:"sehir"`
			ID        string `json:"ilce_id"`
			Name      string `json:"ilce"`
		}
		out := make([]row, 0, len(hits))
		for _, h := range hits {
			out = append(out, row{c.ID, c.Name, h.State.ID, h.State.Name, h.District.ID, h.District.Name})
		}
		return writeJSONOut(e.stdout, out)
	}

	fmt.Fprintln(e.stdout)
	lastState := ""
	for _, h := range hits {
		if h.State.ID != lastState {
			fmt.Fprintf(e.stdout, "  %s\n", e.style.dim(c.Name+" / "+h.State.Name))
			lastState = h.State.ID
		}
		fmt.Fprintf(e.stdout, "    %s  %s\n", e.style.accent(padRight(h.District.ID, 8)), h.District.Name)
	}

	// Direkt verwendbaren Befehl anbieten — spart das Abtippen der ID.
	first := hits[0].District
	tz := tzForCountry(c)
	fmt.Fprintf(e.stdout, "\n  %s\n    namaz konum ekle <ad> --ilce %s --sd %s\n\n",
		e.style.dim("Eklemek için:"), first.ID, tz)
	return nil
}

// allStatesCached prüft, ob für alle Bundesländer bereits Kreislisten
// vorliegen — nur um den Fortschrittshinweis zu unterdrücken.
func (e *env) allStatesCached(states []provider.Place) bool {
	for _, s := range states {
		p := filepath.Join(e.cache.Dir(), fmt.Sprintf("ilceler-%s-%s.json", e.cfg.Provider, s.ID))
		if _, err := os.Stat(p); err != nil {
			return false
		}
	}
	return true
}

// pickPlace findet einen Eintrag per exakter ID oder per gefaltetem Namen.
func pickPlace(list []provider.Place, want string) (provider.Place, bool) {
	if want == "" {
		return provider.Place{}, false
	}
	for _, p := range list {
		if p.ID == want {
			return p, true
		}
	}
	for _, p := range list {
		if foldTurkish(p.Name) == foldTurkish(want) || foldTurkish(p.NameEn) == foldTurkish(want) {
			return p, true
		}
	}
	// Zuletzt Teilstring — damit "baden" auch "BADEN WURTTEMBERG" trifft.
	for _, p := range list {
		if searchMatches(p.Name, want) || searchMatches(p.NameEn, want) {
			return p, true
		}
	}
	return provider.Place{}, false
}

// --- ekle -------------------------------------------------------------------

func locationAdd(e *env, args []string) error {
	fs := e.subFlags("konum ekle")
	district := fs.String("ilce", "", "Diyanet ilçe ID'si (namaz konum ara ile bulunur)")
	tz := fs.String("sd", "", "IANA saat dilimi, örn: Europe/Berlin")
	title := fs.String("baslik", "", "görünen ad (boşsa ilçe adı kullanılır)")
	rest, err := e.parse(fs, args)
	if err != nil {
		return err
	}

	if len(rest) == 0 {
		return fmt.Errorf("konum adı gerekli, örn: namaz konum ekle is --ilce 11021")
	}
	key := rest[0]
	if *district == "" {
		return fmt.Errorf("--ilce gerekli (namaz konum ara <metin> ile bulun)")
	}

	// Zeitzone: --sd hat Vorrang, sonst aus dem Land des Kreises (falls
	// `konum ara` schon lief), sonst die Systemzeitzone.
	zone := *tz
	if zone == "" {
		if country, _, ok := e.cache.DistrictChain(*district); ok {
			zone = tzForCountry(country)
		} else {
			zone = systemTZName()
		}
	}
	if zone == "" {
		return fmt.Errorf("--sd gerekli (sistem saat dilimi belirlenemedi), örn: --sd Europe/Berlin")
	}
	if _, err := time.LoadLocation(zone); err != nil {
		return fmt.Errorf("saat dilimi %q geçersiz: %w", zone, err)
	}

	// Anzeigename: Flag, sonst aus dem Cache der Kreislisten.
	name := *title
	if name == "" {
		if p, ok := e.cache.LookupDistrict(*district); ok {
			name = toTitleTurkish(p.Name)
		}
	}

	cfg := e.cfg
	if cfg.Locations == nil {
		cfg.Locations = map[string]config.Location{}
	}
	_, existed := cfg.Locations[key]
	cfg.Locations[key] = config.Location{DistrictID: *district, Name: name, TZ: zone}

	// Erster Standort überhaupt: gleich zum Vorgabestandort machen.
	if cfg.DefaultLocation == "" || len(cfg.Locations) == 1 {
		cfg.DefaultLocation = key
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	if err := cfg.Save(cfg.Path()); err != nil {
		return err
	}

	verb := "eklendi"
	if existed {
		verb = "güncellendi"
	}
	shown := name
	if shown == "" {
		shown = *district
	}
	fmt.Fprintf(e.stdout, "%s %s %s: %s (%s), %s\n",
		e.style.ok("✓"), key, verb, shown, *district, zone)
	return nil
}

// --- sil --------------------------------------------------------------------

func locationRemove(e *env, args []string) error {
	fs := e.subFlags("konum sil")
	rest, err := e.parse(fs, args)
	if err != nil {
		return err
	}
	if len(rest) == 0 {
		return fmt.Errorf("silinecek konum adı gerekli")
	}
	key := rest[0]

	cfg := e.cfg
	if _, ok := cfg.Locations[key]; !ok {
		return fmt.Errorf("konum %q bulunamadı", key)
	}
	delete(cfg.Locations, key)

	// War es der Vorgabestandort, rückt der alphabetisch erste nach — sonst
	// zeigt jeder folgende Aufruf einen Fehler an.
	if cfg.DefaultLocation == key {
		cfg.DefaultLocation = ""
		if names := cfg.LocationNames(); len(names) > 0 {
			cfg.DefaultLocation = names[0]
		}
	}
	if err := cfg.Save(cfg.Path()); err != nil {
		return err
	}

	fmt.Fprintf(e.stdout, "%s %s silindi\n", e.style.ok("✓"), key)
	if cfg.DefaultLocation != "" && cfg.DefaultLocation != e.cfg.DefaultLocation {
		fmt.Fprintf(e.stdout, "  %s\n", e.style.dim("yeni varsayılan: "+cfg.DefaultLocation))
	}
	return nil
}

// --- varsayilan -------------------------------------------------------------

func locationDefault(e *env, args []string) error {
	fs := e.subFlags("konum varsayilan")
	rest, err := e.parse(fs, args)
	if err != nil {
		return err
	}
	if len(rest) == 0 {
		fmt.Fprintln(e.stdout, e.cfg.DefaultLocation)
		return nil
	}
	key := rest[0]

	cfg := e.cfg
	if _, ok := cfg.Locations[key]; !ok {
		return fmt.Errorf("konum %q bulunamadı (tanımlı: %s)", key, strings.Join(cfg.LocationNames(), ", "))
	}
	cfg.DefaultLocation = key
	if err := cfg.Save(cfg.Path()); err != nil {
		return err
	}
	fmt.Fprintf(e.stdout, "%s varsayılan konum: %s\n", e.style.ok("✓"), key)
	return nil
}

// --- Hilfsfunktionen --------------------------------------------------------

// countryTZ ordnet den Ländern mit Diyanet-Gemeinden ihre Zeitzone zu.
//
// Gebraucht wird das nur für den Vorschlag, den `namaz konum ara` ausgibt.
// Der Umweg über das Land ist hier genauer als die Systemzeitzone: seit
// tzdata 2022b sind viele europäische Zonen zusammengelegt, und macOS
// kanonisiert /etc/localtime auf den Sammelnamen — für Deutschland etwa auf
// "Europe/Oslo". Das ist regeltechnisch identisch, in einer Konfigurationsdatei
// für Weinheim aber verwirrend.
//
// Schlüssel sind gefaltete Ländernamen (siehe foldTurkish). Länder mit
// mehreren Zeitzonen (USA, Kanada, Russland, Australien) fehlen absichtlich —
// dort muss --sd angegeben werden.
var countryTZ = map[string]string{
	"turkiye":      "Europe/Istanbul",
	"almanya":      "Europe/Berlin",
	"avusturya":    "Europe/Vienna",
	"hollanda":     "Europe/Amsterdam",
	"belcika":      "Europe/Brussels",
	"fransa":       "Europe/Paris",
	"isvicre":      "Europe/Zurich",
	"ingiltere":    "Europe/London",
	"danimarka":    "Europe/Copenhagen",
	"isvec":        "Europe/Stockholm",
	"norvec":       "Europe/Oslo",
	"finlandiya":   "Europe/Helsinki",
	"italya":       "Europe/Rome",
	"ispanya":      "Europe/Madrid",
	"polonya":      "Europe/Warsaw",
	"cekya":        "Europe/Prague",
	"macaristan":   "Europe/Budapest",
	"romanya":      "Europe/Bucharest",
	"bulgaristan":  "Europe/Sofia",
	"yunanistan":   "Europe/Athens",
	"kuzey kibris": "Asia/Nicosia",
	"azerbaycan":   "Asia/Baku",
	"bosna hersek": "Europe/Sarajevo",
	"makedonya":    "Europe/Skopje",
	"arnavutluk":   "Europe/Tirane",
	"kosova":       "Europe/Belgrade",
	"estonya":      "Europe/Tallinn",
	"monako":       "Europe/Monaco",
	"lubnan":       "Asia/Beirut",
	"urdun":        "Asia/Amman",
}

// tzForCountry schlägt eine Zeitzone für ein Land vor. Ist das Land nicht
// bekannt, wird die Systemzeitzone verwendet; auch die kann leer sein, dann
// muss --sd angegeben werden.
func tzForCountry(c provider.Place) string {
	if tz, ok := countryTZ[foldTurkish(c.Name)]; ok {
		return tz
	}
	if tz, ok := countryTZ[foldTurkish(c.NameEn)]; ok {
		return tz
	}
	if tz := systemTZName(); tz != "" {
		return tz
	}
	return "Europe/Istanbul"
}

// systemTZName ermittelt die IANA-Zeitzone des Systems.
//
// time.Local hilft nicht weiter: dessen String() liefert je nach Plattform
// nur "Local" oder eine Abkürzung wie "CEST", was time.LoadLocation nicht
// zurücklesen kann. Deshalb der Umweg über TZ bzw. das Symlink-Ziel von
// /etc/localtime — das funktioniert auf macOS wie auf Linux.
func systemTZName() string {
	if tz := os.Getenv("TZ"); tz != "" {
		if _, err := time.LoadLocation(tz); err == nil {
			return tz
		}
	}
	target, err := os.Readlink("/etc/localtime")
	if err != nil {
		return ""
	}
	// z. B. "/var/db/timezone/zoneinfo/Europe/Berlin" (macOS)
	//   oder "../usr/share/zoneinfo/Europe/Berlin"    (Linux)
	const marker = "zoneinfo/"
	i := strings.LastIndex(target, marker)
	if i < 0 {
		return ""
	}
	name := target[i+len(marker):]
	if _, err := time.LoadLocation(name); err != nil {
		return ""
	}
	return name
}

// toTitleTurkish wandelt "WEINHEIM" in "Weinheim".
//
// strings.Title bzw. cases.Title scheitern hier an der türkischen
// i/ı-Regel; für Ortsnamen genügt diese einfache Variante, die den ersten
// Buchstaben jedes Wortes stehen lässt und den Rest kleinschreibt.
func toTitleTurkish(s string) string {
	words := strings.Fields(strings.ToLower(s))
	for i, w := range words {
		r := []rune(w)
		// Türkisches Kleinbuchstaben-i wird zu İ, nicht zu I.
		switch r[0] {
		case 'i':
			r[0] = 'İ'
		case 'ı':
			r[0] = 'I'
		default:
			r[0] = []rune(strings.ToUpper(string(r[0])))[0]
		}
		words[i] = string(r)
	}
	return strings.Join(words, " ")
}
