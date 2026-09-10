// Kommando namaz zeigt die von Diyanet veröffentlichten Gebetszeiten im
// Terminal an.
//
// Bauen:
//
//	go build -o namaz ./cmd/namaz
//
// Siehe README.md für Installation, Konfiguration und die Anbindung an
// Statusleisten.
package main

import (
	// Die Zeitzonendatenbank wird in die Binary eingebettet. Ohne das wäre
	// das Programm darauf angewiesen, dass /usr/share/zoneinfo existiert —
	// was in schlanken Linux-Containern und auf manchen Distributionen nicht
	// der Fall ist. Kostet rund 450 KB und macht die Binary überall
	// lauffähig, was zum Ziel "einfach weitergeben" passt.
	_ "time/tzdata"

	"github.com/samu380/namaz-vakti/internal/cli"
)

func main() {
	cli.Main()
}
