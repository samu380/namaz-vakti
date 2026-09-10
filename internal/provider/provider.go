// Package provider kapselt den Zugriff auf externe Gebetszeit-Dienste.
//
// Der Zugriff läuft über das Interface Provider, damit die konkrete Quelle
// austauschbar bleibt. Aktuell gibt es eine Implementierung (ezanvakti);
// sollte die offizielle Diyanet-API (awqatsalah.diyanet.gov.tr) einmal ohne
// das dortige 5-Requests-Limit nutzbar sein, lässt sie sich daneben stellen,
// ohne dass CLI oder Cache davon etwas mitbekommen.
package provider

import (
	"context"
	"fmt"
	"time"

	"github.com/samu380/namaz-vakti/pkg/vakit"
)

// Place ist ein Eintrag der Diyanet-Standorthierarchie: Land, Bundesland
// oder Kreis. Die IDs sind Diyanet-intern und werden in der Config abgelegt.
type Place struct {
	ID     string `json:"id"`
	Name   string `json:"ad"`
	NameEn string `json:"ad_en"`
}

// Provider liefert Gebetszeiten und die Standorthierarchie.
//
// Die Methoden führen keinerlei Caching durch — das übernimmt das Paket
// cache, das einen Provider umschließt.
type Provider interface {
	// Name ist der Bezeichner, der in Config und JSON-Ausgabe auftaucht.
	Name() string

	// Calendar lädt das verfügbare Zeitfenster (bei ezanvakti rund 32 Tage)
	// für einen Kreis. loc ist die Zeitzone, in der die gelieferten
	// Uhrzeiten interpretiert werden.
	Calendar(ctx context.Context, districtID string, loc *time.Location) (vakit.Calendar, error)

	// Countries listet alle Länder.
	Countries(ctx context.Context) ([]Place, error)

	// States listet die Bundesländer/Provinzen eines Landes.
	States(ctx context.Context, countryID string) ([]Place, error)

	// Districts listet die Kreise eines Bundeslands.
	Districts(ctx context.Context, stateID string) ([]Place, error)
}

// registry hält die bekannten Provider unter ihrem Config-Namen.
var registry = map[string]func() Provider{
	"ezanvakti": func() Provider { return NewEzanVakti("") },
}

// Get liefert den Provider zu einem Namen aus der Config.
func Get(name string) (Provider, error) {
	f, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("bilinmeyen sağlayıcı %q", name)
	}
	return f(), nil
}

// Names listet die verfügbaren Provider-Namen.
func Names() []string {
	out := make([]string, 0, len(registry))
	for n := range registry {
		out = append(out, n)
	}
	return out
}
