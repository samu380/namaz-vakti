# namaz

Diyanet-Gebetszeiten im Terminal. Eine einzelne, statisch gelinkte Go-Binary für macOS und Linux — ohne Runtime, ohne API-Schlüssel, im Normalbetrieb ohne Netzwerkzugriff.

```
  Weinheim · ev                          Pazar, 6 Eylül 2026
                                       24 Rebiulevvel 1448

    İmsak    04:53
    Güneş    06:41
    Öğle     13:29  ▸ şu an
    İkindi   17:06
    Akşam    20:07
    Yatsı    21:41

  Öğle vaktinin bitişine 2 sa 54 dk — İkindi başlıyor (17:06)
```

Gebetsnamen und Ausgabe sind türkisch, Kommandos und Flags reines ASCII (`yarin`, `--cevrimdisi`), damit sie auf einer deutschen Tastatur ohne Umwege tippbar sind. Kommandos mit Diakritika (`yarın`) funktionieren ebenfalls.

## Inhalt

- [Installation](#installation)
- [Erste Schritte](#erste-schritte)
- [Kommandos](#kommandos)
- [Konfiguration](#konfiguration)
- [JSON-Schnittstelle](#json-schnittstelle)
- [Anbindung an Statusleisten](#anbindung-an-statusleisten)
- [Datenquelle und Genauigkeit](#datenquelle-und-genauigkeit)
- [Bekannte Grenzen](#bekannte-grenzen)
- [Entwicklung](#entwicklung)

## Installation

Voraussetzung ist Go 1.22 oder neuer.

```sh
git clone https://github.com/samu380/namaz-vakti
cd namaz-vakti
make install          # nach $(go env GOPATH)/bin
```

Oder nur bauen, ohne zu installieren:

```sh
make build            # erzeugt ./namaz
```

Binaries für alle unterstützten Plattformen (darwin/arm64, darwin/amd64, linux/amd64, linux/arm64):

```sh
make dist             # legt sie unter dist/ ab
```

Die Binaries sind mit `CGO_ENABLED=0` gebaut und enthalten die
Zeitzonendatenbank. Sie laufen damit auch in schlanken Containern, in denen
`/usr/share/zoneinfo` fehlt — einfach kopieren und ausführen.

## Erste Schritte

Ohne Konfiguration startet das Werkzeug mit Weinheim als Standort `ev`:

```sh
namaz
```

Einen eigenen Standort einrichten — die Suche geht über Diyanets
Kreisverzeichnis:

```sh
namaz konum ara koeln
namaz konum ekle ev --ilce 11019
```

Die Suche ist gegenüber Schreibweisen tolerant. Diyanet führt deutsche Orte
ohne Umlaute (`KOLN`, `MUNCHEN`, `DUSSELDORF`) und türkische mit voller
Diakritik (`WEİNHEİM`) — gefunden werden sie so oder so:

| Eingabe | Treffer |
|---|---|
| `koeln`, `köln`, `koln` | `KOLN` |
| `muenchen`, `münchen` | `MUNCHEN` |
| `weinheim` | `WEİNHEİM` |

Standardmäßig wird in Deutschland gesucht; ein anderes Land wählt `--ulke`:

```sh
namaz konum ara uskudar --ulke turkiye
```

Beim ersten Lauf lädt die Suche die Kreislisten des Landes herunter (für
Deutschland 16 Anfragen, gedrosselt, rund 15 Sekunden). Danach liegt alles im
Cache und die Suche läuft rein lokal in Millisekunden.

Einen zweiten Standort für die Arbeit anlegen und benutzen:

```sh
namaz konum ekle is --ilce 11021
namaz -k is
```

## Kommandos

### `namaz` — Tagesansicht

Ohne Argumente: die sechs Zeitmarken des heutigen Tages, das laufende
Gebetsfenster hervorgehoben, darunter die Countdown-Zeile.

Der Countdown folgt einer Regel: **die Deadline steht immer oben.** Endet das
laufende Fenster genau dann, wenn das nächste Gebet beginnt — der Normalfall
bei Öğle→İkindi, İkindi→Akşam, Akşam→Yatsı und Yatsı→Sabah —, sagt eine
einzige Zeile beides:

```
  Öğle vaktinin bitişine 2 sa 54 dk — İkindi başlıyor (17:06)
```

Nur im Sabah-Fenster fallen die beiden Zahlen auseinander: es endet mit Güneş,
das nächste Gebet (Öğle) beginnt aber erst Stunden später. Dann werden es zwei
Zeilen:

```
  Sabah vaktinin bitişine 1 sa 11 dk (06:41)
  Sonraki vakit: Öğle 13:29 · 7 sa 59 dk
```

Zwischen Güneş und Öğle ist überhaupt kein Gebetsfenster aktiv:

```
  Şu an farz namaz vakti değil
  Sonraki vakit: Öğle 13:29 · 4 sa 29 dk
```

Läuft eine Kerahat, kommt eine Warnzeile hinzu:

```
  Akşam vaktinin bitişine 11 dk — Yatsı başlıyor (20:07)
  ⚠ Kerahat vakti (İsfirar) — bitişine 11 dk
```

| Flag | Wirkung |
|---|---|
| `--kerahat-yok` | Kerahat-Zeile ausblenden |

### `namaz yarin` und `namaz tarih <YYYY-AA-GG>`

Dieselbe Tabelle für einen anderen Tag, ohne Countdown — für einen anderen Tag
als heute wäre eine Restzeit sinnlos.

```sh
namaz yarin
namaz tarih 2026-09-20
```

Beschränkt auf das Zeitfenster, das die API liefert (rund 32 Tage ab
vorgestern), siehe [Bekannte Grenzen](#bekannte-grenzen).

### `namaz sonraki` — Einzeiler

Genau eine Zeile, sonst nichts. Gedacht für Statusleisten, tmux und Skripte.

```sh
$ namaz sonraki
İkindi 17:06 · 2 sa 54 dk

$ namaz sonraki --kisa
İkindi 2:54
```

| Flag | Wirkung |
|---|---|
| `--bicim <şablon>` | eigene Vorlage, siehe unten |
| `--kisa` | kompakt: `İkindi 2:54` |
| `--saniye` | Countdown sekundengenau |

Platzhalter für `--bicim`:

| Platzhalter | Beispiel |
|---|---|
| `{vakit}` | `İkindi` |
| `{saat}` | `17:06` |
| `{kalan}` | `2 sa 54 dk` |
| `{kalan_ks}` | `2:54` |
| `{kalan_dk}` | `174` |
| `{kalan_sn}` | `10440` |
| `{konum}` | `Weinheim` |
| `{kerahat}` | `İsfirar` (leer, wenn keine läuft) |

```sh
$ namaz sonraki --bicim "{vakit} {kalan_ks}"
İkindi 2:54
```

### `namaz kerahat`

Die drei Kerahat-Fenster eines Tages am Stück. Die Tagesansicht meldet nur die
gerade laufende, damit sie ruhig bleibt; dieses Kommando ist für den Blick nach
vorn.

```
$ namaz kerahat

  Weinheim · ev                          Pazar, 6 Eylül 2026
                                       24 Rebiulevvel 1448

    İşrak     06:41 – 07:26   güneş doğduktan sonra
    İstiva    12:44 – 13:29   öğleden önce
    İsfirar   19:22 – 20:07   akşamdan önce  ▸ şu an, 11 dk kaldı
```

Ein optionales Datum ist möglich: `namaz kerahat 2026-09-20`.

### `namaz konum` — Standorte

| Unterkommando | Zweck |
|---|---|
| `namaz konum` bzw. `konum liste` | konfigurierte Standorte, Vorgabe markiert |
| `namaz konum ara <metin>` | Diyanet-Kreisverzeichnis durchsuchen |
| `namaz konum ekle <ad> --ilce <id>` | Standort anlegen oder ändern |
| `namaz konum sil <ad>` | Standort entfernen |
| `namaz konum varsayilan [<ad>]` | Vorgabestandort setzen bzw. anzeigen |

```
$ namaz konum
    * ev         Weinheim             10532     Europe/Berlin
      is         Mannheim             11021     Europe/Berlin

  * = varsayılan konum
```

Flags von `konum ara`:

| Flag | Wirkung |
|---|---|
| `--ulke <ad\|id>` | Land, in dem gesucht wird (Vorgabe aus `varsayilan_ulke`) |

Flags von `konum ekle`:

| Flag | Wirkung |
|---|---|
| `--ilce <id>` | Diyanet-Kreis-ID (zwingend) |
| `--sd <zone>` | IANA-Zeitzone, z. B. `Europe/Berlin` |
| `--baslik <ad>` | Anzeigename |

Werden `--sd` oder `--baslik` weggelassen, ermittelt das Werkzeug sie aus dem
Cache: den Anzeigenamen aus dem Kreisverzeichnis, die Zeitzone aus dem Land des
Kreises. Beides setzt voraus, dass vorher `namaz konum ara` lief; sonst wird die
Systemzeitzone verwendet.

### `namaz ayar` — Konfiguration

```
$ namaz ayar

  Yapılandırma: /Users/du/.config/namaz-vakti/config.toml

    varsayilan_konum      ev               (yapılandırma)
    varsayilan_ulke       ALMANYA          (yapılandırma)
    saglayici             ezanvakti        (yapılandırma)
    kible_saati           false            (yapılandırma)
    kerahat.israk         45 dk            (yapılandırma)
    kerahat.istiva        45 dk            (yapılandırma)
    kerahat.isfirar       45 dk            (yapılandırma)
    kerahat.uyari         icerideyken      (yapılandırma)
    duzeltme.imsak        +0 dk            (varsayılan)
    ...

  Önbellek: /Users/du/.cache/namaz-vakti
    ev          2026-09-05 – 2026-10-06   güncellendi: az önce
    is          2026-09-01 – 2026-10-02   güncellendi: 1 sa önce
```

Die Spalte rechts sagt, woher jeder Wert stammt. Bei vier Präzedenzebenen
(Flag > Umgebung > Datei > Vorgabe) ist das die einzige Möglichkeit zu sehen,
ob etwa ein gesetzter İkindi-Offset überhaupt greift.

`namaz ayar yol` gibt nur den Pfad aus, sodass er sich weiterverwenden lässt:

```sh
$EDITOR "$(namaz ayar yol)"
```

### Globale Flags

Gelten für jedes Kommando und dürfen davor oder danach stehen
(`namaz -k is sonraki` und `namaz sonraki -k is` sind gleichwertig).

| Flag | Wirkung |
|---|---|
| `-k`, `--konum <ad>` | Standort wählen |
| `--json` | JSON-Ausgabe |
| `--renk <oto\|var\|yok>` | Farben; `oto` nur bei Terminal-Ausgabe |
| `--yenile` | Cache ignorieren, frisch von der API laden |
| `--cevrimdisi` | Niemals ans Netz, nur Cache |
| `--ayar <yol>` | Alternative Konfigurationsdatei |
| `-y`, `--yardim` | Hilfe |
| `--surum` | Version |

Farben werden bei `oto` nur ausgegeben, wenn die Ausgabe wirklich in ein
Terminal geht — `namaz | grep` und `namaz > datei.txt` bleiben frei von
Steuerzeichen. `NO_COLOR` und `TERM=dumb` werden respektiert.

## Konfiguration

Standardpfad ist `~/.config/namaz-vakti/config.toml`, unter macOS wie unter
Linux (bewusst nicht `~/Library/Application Support` — so bleibt die
Konfiguration zwischen beiden Systemen austauschbar). Überschreibbar per
`--ayar` oder `$NAMAZ_CONFIG`; `$XDG_CONFIG_HOME` wird beachtet.

```toml
varsayilan_konum = "ev"
varsayilan_ulke  = "ALMANYA"   # Vorgabe für `konum ara`
saglayici        = "ezanvakti"
kible_saati      = false       # Kıble saati in der Tagesansicht zeigen

[konumlar.ev]
ilce_id     = "10532"
ad          = "Weinheim"
saat_dilimi = "Europe/Berlin"

[konumlar.is]
ilce_id     = "11021"
ad          = "Mannheim"
saat_dilimi = "Europe/Berlin"

[kerahat]
israk   = 45              # Minuten nach Güneş
istiva  = 45              # Minuten vor Öğle
isfirar = 45              # Minuten vor Akşam
uyari   = "icerideyken"   # "icerideyken" | "erken"

[duzeltme]                # Minutenkorrektur je Vakit
ikindi = 0                # asr-ı sani: siehe unten
```

### `kerahat.uyari`

- `icerideyken` (Vorgabe) — es wird gewarnt, sobald eine Kerahat läuft.
- `erken` — es wird schon vorher gewarnt, sobald eine Kerahat ins laufende
  Gebetsfenster fällt.

Praktisch betrifft das nur İkindi. İsfirar ist das einzige Kerahat-Fenster, das
in ein Gebetsfenster hineinragt: İkindi bleibt darin gültig, ist aber mekruh.
Die „harte" Deadline ist Akşam, die „weiche" der Beginn von İsfirar — 45
Minuten davor.

### `duzeltme`

Verschiebt einzelne Vakit-Zeiten um Minuten. Die Kerahat-Fenster verschieben
sich mit, weil sie aus den Vakit-Zeiten abgeleitet werden; die astronomischen
Angaben (Sonnenauf-/-untergang, Kıble saati) bleiben unberührt.

Gedacht ist das vor allem für İkindi — siehe
[Datenquelle und Genauigkeit](#der-ikindi-vorbehalt).

### Cache

Standardpfad `~/.cache/namaz-vakti`, überschreibbar per `$NAMAZ_CACHE`;
`$XDG_CACHE_HOME` wird beachtet.

Die API liefert rund 32 Tage am Stück, ein Abruf deckt also gut vier Wochen ab.
Ein Neuladen erfolgt nur, wenn der gesuchte Tag (oder der Folgetag — das
Yatsı-Fenster reicht bis zum İmsak des nächsten Tages) nicht mehr enthalten
ist. Im Alltag arbeitet das Werkzeug damit offline und ohne Latenz.

Uhrzeiten werden als Wandzeit (`"17:06"`) gespeichert, nicht als absoluter
Zeitstempel. Beim Laden wird die Zeitzone erneut angewendet — dadurch bleibt
ein Kalender korrekt, der über die Sommerzeitumstellung hinausreicht.

Cache leeren:

```sh
rm -rf "$(namaz ayar yol | xargs dirname | sed 's/config/cache/')"   # oder einfach:
rm -rf ~/.cache/namaz-vakti
```

## JSON-Schnittstelle

`--json` funktioniert an jedem Kommando und ist als **stabile Schnittstelle**
gedacht. Alle Schlüssel sind reines ASCII, damit `jq`-Ausdrücke und Skripte
ohne Quoting-Ärger auskommen. Jede Uhrzeit erscheint doppelt: als `"HH:MM"`
zum Anzeigen und als ISO-8601-Zeitstempel mit Zonenoffset zum Rechnen.

```jsonc
{
  "konum":  { "anahtar": "ev", "ad": "Weinheim",
              "ilce_id": "10532", "saat_dilimi": "Europe/Berlin" },
  "tarih":  { "miladi": "2026-09-06", "gun": "Pazar",
              "hicri": "24 Rebiulevvel 1448" },
  "vakitler": [
    { "ad": "imsak",  "etiket": "İmsak",  "saat": "04:53",
      "iso": "2026-09-06T04:53:00+02:00" }
    // ... gunes, ogle, ikindi, aksam, yatsi
  ],
  "gunes_dogus": "06:48",     // astronomisch, ohne Temkin
  "gunes_batis": "20:00",
  "kible_saati": "10:40",
  "su_anki":  { "vakit": "Öğle", "baslangic_vakti": "ogle",
                "bitis": "17:06", "bitis_iso": "2026-09-06T17:06:00+02:00",
                "kalan_saniye": 10440 },
  "sonraki":  { "ad": "ikindi", "vakit": "İkindi", "saat": "17:06",
                "iso": "2026-09-06T17:06:00+02:00", "kalan_saniye": 10440 },
  "kerahat":  [ { "tur": "isfirar", "etiket": "İsfirar",
                  "baslangic": "19:22", "bitis": "20:07",
                  "baslangic_iso": "...", "bitis_iso": "...",
                  "aktif": false } ],
  "kaynak":   { "saglayici": "ezanvakti", "onbellekten": true,
                "alindi": "2026-09-06T08:12:03+02:00" }
}
```

`su_anki` ist `null`, wenn kein Gebetsfenster aktiv ist (zwischen Güneş und
Öğle). Bei Tagen außer heute sind `su_anki` und `sonraki` beide `null`.

```sh
namaz --json | jq -r '.sonraki | "\(.vakit) in \(.kalan_saniye/60|floor) Minuten"'
namaz konum --json | jq -r '.[] | select(.varsayilan) | .ad'
```

## Anbindung an Statusleisten

### SwiftBar / xbar (macOS)

Als `namaz.1m.sh` in den SwiftBar-Plugin-Ordner legen und ausführbar machen:

```sh
#!/bin/bash
export PATH="$HOME/go/bin:/opt/homebrew/bin:$PATH"

echo "$(namaz sonraki --bicim '{vakit} {kalan_ks}')"
echo "---"
namaz --renk yok | sed 's/^/  /'
echo "---"
echo "Yenile | refresh=true"
```

Der Aufruf kostet keinen Netzwerkzugriff, solange der Cache den Tag abdeckt —
ein Aufruf pro Minute ist damit unproblematisch.

### tmux

In `~/.tmux.conf`:

```tmux
set -g status-right '#(namaz sonraki --bicim "{vakit} {kalan_ks}") | %H:%M'
set -g status-interval 60
```

### Waybar (Linux)

```json
"custom/namaz": {
  "exec": "namaz sonraki --bicim '{vakit} {kalan_ks}'",
  "interval": 60
}
```

### Shell-Prompt

```sh
# in ~/.zshrc
RPROMPT='%F{240}$(namaz sonraki --kisa)%f'
```

## Datenquelle und Genauigkeit

Die Zeiten kommen von [ezanvakti.emushaf.net](https://ezanvakti.emushaf.net/),
einem Spiegel der von Diyanet veröffentlichten Gebetszeiten. Kein API-Schlüssel,
kein Rate-Limit-Antrag — dieselben Zahlen wie auf
`namazvakti.diyanet.gov.tr` und in der offiziellen App.

Nicht verwendet wird die offizielle Diyanet-API
([awqatsalah.diyanet.gov.tr](https://awqatsalah.diyanet.gov.tr/)): sie
verlangt ein unterschriebenes Antragsformular und begrenzt auf 5 Requests pro
Endpunkt, was eine Weitergabe des Werkzeugs praktisch ausschließt. Das
Interface `provider.Provider` ist so geschnitten, dass sie sich bei Bedarf
daneben stellen ließe.

### Temkin

Diyanet rechnet eine Sicherheitsmarge ein. In Weinheim liegt Güneş 7 Minuten
*vor* dem astronomischen Sonnenaufgang und Akşam 7 Minuten *danach*. Genau
deshalb rechnet dieses Werkzeug die Zeiten nicht selbst, sondern übernimmt sie:
eine Eigenberechnung träfe die gedruckten Kalender nicht.

Die astronomischen Werte stehen als `gunes_dogus` und `gunes_batis` in der
JSON-Ausgabe zur Verfügung.

### Kerahat

Die API liefert keine Kerahat-Zeiten. Sie werden aus den veröffentlichten
Vakit-Zeiten abgeleitet:

| Fenster | Ableitung |
|---|---|
| İşrak | Güneş bis Güneş + `kerahat.israk` |
| İstiva | Öğle − `kerahat.istiva` bis Öğle |
| İsfirar | Akşam − `kerahat.isfirar` bis Akşam |

45 Minuten entspricht der gängigen türkischen Kalenderkonvention. Verankert
wird bewusst an den Diyanet-Zeiten und nicht an den astronomischen, sodass die
Fenster Diyanets Temkin-Marge erben.

### Der İkindi-Vorbehalt

**Diyanets İkindi ist nicht der hanefitische asr-ı sani.**

Nachgerechnet für Weinheim (49,55° N) am 6. September 2026:

| | Zeit |
|---|---|
| asr-ı evvel, Schattenfaktor 1 (berechnet) | ~17:02 |
| **Diyanet İkindi (veröffentlicht)** | **17:06** |
| asr-ı sani, Schattenfaktor 2 (berechnet) | ~17:58 |

Diyanet verwendet Schattenfaktor 1 zuzüglich Temkin. Das ist die Zeit, nach der
auch die Moscheen gehen. Wer den hanefitischen asr-ı sani bevorzugt, verschiebt
İkindi über die Konfiguration:

```toml
[duzeltme]
ikindi = 52
```

Der Betrag ist jahreszeitabhängig (im Sommer größer, im Winter kleiner) — eine
feste Minutenzahl ist also eine Näherung. Eine exakte asr-ı-sani-Berechnung
bräuchte Koordinaten, die eine İlçe-ID-basierte API nicht liefert.

## Bekannte Grenzen

**Zeitfenster von 32 Tagen.** Die API kennt nur einen Endpunkt ohne
Datumsparameter (`/vakitler/{ilce}`) und liefert ein rollierendes Fenster von
rund 32 Tagen ab vorgestern. `namaz tarih` funktioniert nur darin; für
Dezember-Zeiten im September gibt es eine klare Fehlermeldung mit dem
abgedeckten Zeitraum. Eine Monats- oder Jahresansicht (İmsakiye) würde eine
zweite Datenquelle brauchen.

**Erster Lauf von `konum ara`** lädt die Kreislisten eines Landes einzeln
herunter, gedrosselt auf eine Anfrage pro 250 ms, um Cloudflares Rate-Limit
nicht zu treffen. Für Deutschland dauert das rund 15 Sekunden. Danach ist es
gecacht.

**Keine Benachrichtigungen.** Ein Prozess, der im Hintergrund läuft und zum
Vakit benachrichtigt, ist ein Daemon mit allem, was dazugehört. Das CLI liefert
die Daten, die Benachrichtigung gehört in ein Menüleisten-Tool.

**Hohe Breitengrade.** Wie Diyanet damit umgeht, ist Diyanets Entscheidung —
das Werkzeug übernimmt die Zahlen unverändert. Für Standorte, an denen im
Sommer kein echtes Isha eintritt, gelten also Diyanets Konventionen.

## Entwicklung

```
cmd/namaz/           Einstiegspunkt, bettet die Zeitzonendatenbank ein
internal/cli/        Kommandos, Flags, Text- und JSON-Ausgabe
internal/config/     TOML-Konfiguration, Standortverwaltung
internal/cache/      Ablage der API-Antworten, DST-sicher
internal/provider/   Zugriff auf externe Dienste (Interface + ezanvakti)
pkg/vakit/           Domänenmodell: Zeitmarken, Gebetsfenster, Kerahat
```

`pkg/vakit` ist frei von CLI-, Netzwerk- und Konfigurationsbelangen und lässt
sich aus anderen Go-Programmen importieren. Funktionen, die „jetzt" brauchen,
bekommen die Zeit hineingereicht statt `time.Now()` aufzurufen — nur dadurch
lassen sich alle Tageszeitpunkte reproduzierbar testen.

```sh
make test             # go test ./...
make vet
make fmt
```

Die Tests in `pkg/vakit` arbeiten mit echten Diyanet-Daten für Weinheim und
prüfen unter anderem:

- die Countdown-Logik zu neun Tageszeitpunkten, inklusive der Fälle über
  Mitternacht und der Vormittagslücke,
- dass Güneş nie als „nächstes Gebet" auftaucht,
- die Temkin-Annahme von 7 Minuten,
- die Ableitung und Grenzen der Kerahat-Fenster,
- die Minutenkorrektur.

`internal/cache` prüft zusätzlich den Rundlauf über die Sommerzeitumstellung
vom 25. Oktober 2026.

### Eine weitere Datenquelle anbinden

`provider.Provider` verlangt vier Methoden (`Calendar`, `Countries`, `States`,
`Districts`). Eine neue Implementierung wird in `provider.registry` eingetragen
und ist dann über `saglayici` in der Konfiguration wählbar. Cache und CLI
bleiben unverändert.

## Abhängigkeiten

Eine: [`github.com/BurntSushi/toml`](https://github.com/BurntSushi/toml) für
die Konfigurationsdatei. Alles Übrige ist Standardbibliothek.

## Lizenz

MIT
