package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

type multiFlag []string

func (m *multiFlag) String() string       { return strings.Join(*m, ",") }
func (m *multiFlag) Set(val string) error { *m = append(*m, val); return nil }

var (
	intervalMs int
	cmdStr     string
)

func main() {
	var netDevs multiFlag
	var diskDevs multiFlag
	var monitorCPU bool
	var monitorRAM bool

	flag.Var(&netDevs, "n", "Network-Device(s), mutliple allowed")
	flag.Var(&diskDevs, "d", "Disk-Device(s), mutliple allowed")
	flag.BoolVar(&monitorCPU, "c", false, "enable CPU monitoring")
	flag.BoolVar(&monitorRAM, "r", false, "enable RAM monitoring")
	flag.IntVar(&intervalMs, "i", 1000, "sampling rate in Milliseconds")
	flag.StringVar(&cmdStr, "cmd", "", "console Command")
	flag.Parse()

	if len(netDevs) == 0 && len(diskDevs) == 0 && !monitorCPU && !monitorRAM {
		fmt.Println("Usage: perfAnalyzer [Options]")
		fmt.Println()
		fmt.Println("Options:")
		fmt.Println("  -n <device>   monitor specified Network Device")
		fmt.Println("  -d <device>   monitor specified Disk")
		fmt.Println("  -c            monitor CPU")
		fmt.Println("  -r            monitor RAM")
		fmt.Println("  -i            sampling rate in ms (default:1000)")
		fmt.Println("  -cmd          execute command while monitoring with perfAnalyzer")
		fmt.Println("")
		fmt.Println("Example:")
		fmt.Println("  perfAnalyzer -n enp1s0 -d sda1 -c -r -i 2000 -cmd 'cp /some/random/file /to/some/random/path'")
		return
	}

	fmt.Printf("Used Network Interface: %s\n", netDevs)
	fmt.Printf("Use Disk: %s\n", diskDevs)
	fmt.Println()

	interval := time.Duration(intervalMs) * time.Millisecond
	analyzer := NewPerformanceAnalyzer(interval, netDevs, diskDevs, monitorCPU, monitorRAM)

	// Signal-Handler für sauberes Beenden
	go func() {
		var input string
		fmt.Scanln(&input) // Warte auf Enter
		analyzer.Stop()
	}()

	if cmdStr != "" {
		// Kommando aufsplitten
		parts := strings.Fields(cmdStr)
		cmd := exec.Command(parts[0], parts[1:]...)

		// Stdout/Stderr weiterleiten, damit der Nutzer es sieht
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		// Kommando starten
		if err := cmd.Start(); err != nil {
			fmt.Printf("Fehler beim Starten des Kommandos: %v", err)
		}

		// Analyzer starten
		go analyzer.Start()

		// Warten bis das Kommando fertig ist
		if err := cmd.Wait(); err != nil {
			fmt.Printf("Kommando beendet mit Fehler: %v", err)
		}

		// Analyzer beenden
		analyzer.Stop()

	} else {
		// Kein Kommando: Analyzer einfach laufen lassen
		analyzer.Start()
	}
}

// Start beginnt mit der Überwachung
func (pa *PerformanceAnalyzer) Start() {
	pa.mu.Lock()
	pa.running = true
	pa.mu.Unlock()

	ticker := time.NewTicker(pa.interval)
	defer ticker.Stop()

	fmt.Printf("Performance Analyzer gestartet - überwacht %s (Netzwerk) und %s (Festplatte)\n",
		pa.networkDevs, pa.diskDevs)
	fmt.Println("Drücke <Enter> zum Beenden")
	fmt.Println()

	// Sofort eine erste Messung machen
	stat := pa.collectStats()
	pa.addStat(stat)
	pa.printCurrentStats(stat)

	for pa.isRunning() {
		select {
		case <-ticker.C:
			stat := pa.collectStats()
			pa.addStat(stat)
			pa.printCurrentStats(stat)
		default:
			time.Sleep(50 * time.Millisecond)

		}
	}
}

// Stop beendet die Überwachung
func (pa *PerformanceAnalyzer) Stop() {
	pa.mu.Lock()
	pa.running = false
	pa.mu.Unlock()
	pa.PrintSummary()
	pa.PrintAnalysis()
}

func (pa *PerformanceAnalyzer) isRunning() bool {
	pa.mu.RLock()
	defer pa.mu.RUnlock()
	return pa.running
}
