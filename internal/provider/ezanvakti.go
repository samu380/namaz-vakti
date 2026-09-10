package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/samu380/namaz-vakti/pkg/vakit"
)

// DefaultEzanVaktiBaseURL ist der öffentliche Spiegel der von Diyanet
// veröffentlichten Gebetszeiten. Er benötigt keinen API-Schlüssel und liefert
// dieselben Zahlen wie namazvakti.diyanet.gov.tr, inklusive der dort
// eingerechneten Temkin-Marge.
const DefaultEzanVaktiBaseURL = "https://ezanvakti.emushaf.net"

// minRequestInterval ist der Mindestabstand zwischen zwei Anfragen.
//
// Nötig, weil `namaz konum ara` für ein Land die Kreisliste jedes
// Bundeslands holt — für Deutschland 16 Anfragen. Ohne Drosselung greift
// Cloudflares Rate-Limit (HTTP 429) und ein Teil der Listen fehlt.
const minRequestInterval = 250 * time.Millisecond

// maxRetries ist die Zahl der Wiederholversuche bei 429 und 5xx.
const maxRetries = 3

// ezanVakti implementiert Provider gegen den ezanvakti-Spiegel.
type ezanVakti struct {
	baseURL string
	http    *http.Client

	// mu und lastRequest bilden die Drosselung. Der Provider wird pro
	// Programmlauf einmal erzeugt und ist damit der richtige Ort dafür.
	mu          sync.Mutex
	lastRequest time.Time
}

// NewEzanVakti erzeugt den Provider. Ein leeres baseURL verwendet den
// Standard-Endpunkt; der Parameter existiert vor allem für Tests.
func NewEzanVakti(baseURL string) Provider {
	if baseURL == "" {
		baseURL = DefaultEzanVaktiBaseURL
	}
	return &ezanVakti{
		baseURL: strings.TrimRight(baseURL, "/"),
		// Großzügiges, aber endliches Timeout: das Tool soll bei einem
		// hängenden Dienst nicht ewig blockieren, sondern auf den Cache
		// zurückfallen können.
		http: &http.Client{Timeout: 20 * time.Second},
	}
}

func (e *ezanVakti) Name() string { return "ezanvakti" }

// throttle wartet, bis der Mindestabstand zur letzten Anfrage erreicht ist.
func (e *ezanVakti) throttle(ctx context.Context) error {
	e.mu.Lock()
	wait := minRequestInterval - time.Since(e.lastRequest)
	e.lastRequest = time.Now().Add(max(0, wait))
	e.mu.Unlock()

	if wait <= 0 {
		return nil
	}
	select {
	case <-time.After(wait):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// getJSON führt ein GET aus und dekodiert die Antwort nach out.
//
// Bei 429 (Rate-Limit) und 5xx wird mit wachsender Wartezeit wiederholt;
// ein per Retry-After angegebener Wert hat Vorrang. Andere Fehler werden
// sofort gemeldet — ein 404 wird durch Warten nicht besser.
func (e *ezanVakti) getJSON(ctx context.Context, path string, out any) error {
	url := e.baseURL + path
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// 1s, 2s, 4s — plus die Angabe des Servers, falls vorhanden.
			backoff := time.Duration(1<<(attempt-1)) * time.Second
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		if err := e.throttle(ctx); err != nil {
			return err
		}

		retryable, err := e.tryGet(ctx, url, out)
		if err == nil {
			return nil
		}
		lastErr = err
		if !retryable {
			return err
		}
	}
	return fmt.Errorf("%d denemeden sonra başarısız: %w", maxRetries+1, lastErr)
}

// tryGet führt einen einzelnen Versuch aus. retryable meldet, ob ein
// Wiederholversuch sinnvoll ist.
func (e *ezanVakti) tryGet(ctx context.Context, url string, out any) (retryable bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, fmt.Errorf("istek oluşturulamadı: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "namaz-vakti/1.0 (+https://github.com/samu380/namaz-vakti)")

	resp, err := e.http.Do(req)
	if err != nil {
		// Netzwerkfehler sind oft vorübergehend.
		return true, fmt.Errorf("%s adresine ulaşılamadı: %w", url, err)
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusOK:
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return false, fmt.Errorf("%s: yanıt çözümlenemedi: %w", url, err)
		}
		return false, nil

	case resp.StatusCode == http.StatusTooManyRequests, resp.StatusCode >= 500:
		// Vom Server vorgegebene Wartezeit berücksichtigen.
		if ra := resp.Header.Get("Retry-After"); ra != "" {
			if secs, convErr := strconv.Atoi(ra); convErr == nil && secs > 0 && secs <= 30 {
				select {
				case <-time.After(time.Duration(secs) * time.Second):
				case <-ctx.Done():
					return false, ctx.Err()
				}
			}
		}
		return true, fmt.Errorf("%s: HTTP %d", url, resp.StatusCode)

	default:
		// Antwortkörper mitlesen, aber begrenzen — Fehlerseiten können groß sein.
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
		return false, fmt.Errorf("%s: HTTP %d %s", url, resp.StatusCode, strings.TrimSpace(string(snippet)))
	}
}

// --- Standorthierarchie -----------------------------------------------------

type apiCountry struct {
	UlkeAdi   string `json:"UlkeAdi"`
	UlkeAdiEn string `json:"UlkeAdiEn"`
	UlkeID    string `json:"UlkeID"`
}

type apiState struct {
	SehirAdi   string `json:"SehirAdi"`
	SehirAdiEn string `json:"SehirAdiEn"`
	SehirID    string `json:"SehirID"`
}

type apiDistrict struct {
	IlceAdi   string `json:"IlceAdi"`
	IlceAdiEn string `json:"IlceAdiEn"`
	IlceID    string `json:"IlceID"`
}

func (e *ezanVakti) Countries(ctx context.Context) ([]Place, error) {
	var raw []apiCountry
	if err := e.getJSON(ctx, "/ulkeler", &raw); err != nil {
		return nil, err
	}
	out := make([]Place, len(raw))
	for i, c := range raw {
		out[i] = Place{ID: c.UlkeID, Name: c.UlkeAdi, NameEn: c.UlkeAdiEn}
	}
	return out, nil
}

func (e *ezanVakti) States(ctx context.Context, countryID string) ([]Place, error) {
	var raw []apiState
	if err := e.getJSON(ctx, "/sehirler/"+countryID, &raw); err != nil {
		return nil, err
	}
	out := make([]Place, len(raw))
	for i, s := range raw {
		out[i] = Place{ID: s.SehirID, Name: s.SehirAdi, NameEn: s.SehirAdiEn}
	}
	return out, nil
}

func (e *ezanVakti) Districts(ctx context.Context, stateID string) ([]Place, error) {
	var raw []apiDistrict
	if err := e.getJSON(ctx, "/ilceler/"+stateID, &raw); err != nil {
		return nil, err
	}
	out := make([]Place, len(raw))
	for i, d := range raw {
		out[i] = Place{ID: d.IlceID, Name: d.IlceAdi, NameEn: d.IlceAdiEn}
	}
	return out, nil
}

// --- Gebetszeiten -----------------------------------------------------------

// apiDay bildet einen Tag der /vakitler-Antwort ab.
//
// Bewusst nicht übernommen werden GreenwichOrtalamaZamani und die
// Iso8601-Felder: Ersteres stand im Test für Weinheim auf 3.0 (Türkei) statt
// 2.0, Letztere tragen teils ebenfalls den türkischen Offset. Wir verlassen
// uns ausschließlich auf die reinen "HH:MM"-Strings und hängen die in der
// Config hinterlegte IANA-Zeitzone an.
type apiDay struct {
	Imsak      string `json:"Imsak"`
	Gunes      string `json:"Gunes"`
	Ogle       string `json:"Ogle"`
	Ikindi     string `json:"Ikindi"`
	Aksam      string `json:"Aksam"`
	Yatsi      string `json:"Yatsi"`
	GunesDogus string `json:"GunesDogus"`
	GunesBatis string `json:"GunesBatis"`
	KibleSaati string `json:"KibleSaati"`

	MiladiTarihKisa string `json:"MiladiTarihKisa"` // "06.09.2026"
	HicriTarihKisa  string `json:"HicriTarihKisa"`  // "24.3.1448"
	HicriTarihUzun  string `json:"HicriTarihUzun"`  // "24 Rebiulevvel 1448"
}

func (e *ezanVakti) Calendar(ctx context.Context, districtID string, loc *time.Location) (vakit.Calendar, error) {
	var raw []apiDay
	if err := e.getJSON(ctx, "/vakitler/"+districtID, &raw); err != nil {
		return vakit.Calendar{}, err
	}
	if len(raw) == 0 {
		return vakit.Calendar{}, fmt.Errorf("ilçe %s için vakit bulunamadı", districtID)
	}

	cal := vakit.Calendar{Location: loc, Days: make([]vakit.Day, 0, len(raw))}
	for _, r := range raw {
		day, err := r.toDay(loc)
		if err != nil {
			return vakit.Calendar{}, fmt.Errorf("ilçe %s, %s: %w", districtID, r.MiladiTarihKisa, err)
		}
		cal.Days = append(cal.Days, day)
	}
	return cal, nil
}

// toDay setzt einen API-Tag in das Domänenmodell um.
func (r apiDay) toDay(loc *time.Location) (vakit.Day, error) {
	// Datum im Format "06.09.2026", in der Zeitzone des Standorts.
	date, err := time.ParseInLocation("02.01.2006", r.MiladiTarihKisa, loc)
	if err != nil {
		return vakit.Day{}, fmt.Errorf("tarih çözümlenemedi %q: %w", r.MiladiTarihKisa, err)
	}

	// clock hängt eine "HH:MM"-Angabe an das Kalenderdatum. Über
	// time.Date wird dabei automatisch die zum Zeitpunkt gültige
	// Sommer-/Winterzeit angewendet.
	clock := func(field, hhmm string) (time.Time, error) {
		t, err := time.Parse("15:04", hhmm)
		if err != nil {
			return time.Time{}, fmt.Errorf("%s saati çözümlenemedi %q: %w", field, hhmm, err)
		}
		return time.Date(date.Year(), date.Month(), date.Day(),
			t.Hour(), t.Minute(), 0, 0, loc), nil
	}

	day := vakit.Day{
		Date:       date,
		HijriShort: r.HicriTarihKisa,
		HijriLong:  r.HicriTarihUzun,
	}

	// Die sechs Vakit-Marken in der Reihenfolge von vakit.AllKinds.
	fields := [vakit.KindCount]struct {
		name string
		val  string
	}{
		{"İmsak", r.Imsak},
		{"Güneş", r.Gunes},
		{"Öğle", r.Ogle},
		{"İkindi", r.Ikindi},
		{"Akşam", r.Aksam},
		{"Yatsı", r.Yatsi},
	}
	for i, f := range fields {
		t, err := clock(f.name, f.val)
		if err != nil {
			return vakit.Day{}, err
		}
		day.Times[i] = t
	}

	// Zusatzangaben. Sie sind nicht kritisch für die Kernfunktion, deshalb
	// werden Parse-Fehler hier toleriert und der Wert bleibt die Nullzeit.
	if t, err := clock("GunesDogus", r.GunesDogus); err == nil {
		day.Sunrise = t
	}
	if t, err := clock("GunesBatis", r.GunesBatis); err == nil {
		day.Sunset = t
	}
	if t, err := clock("KibleSaati", r.KibleSaati); err == nil {
		day.QiblaTime = t
	}

	return day, nil
}
