package main

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"time"
)

type PerformanceIssue struct {
	Component   string        // "CPU", "RAM", "Network", "Disk"
	Device      string        // z.B. "sda", "eth0" oder leer für CPU/RAM
	Severity    string        // "Critical", "Warning", "Info"
	Description string        // Beschreibung des Problems
	MaxValue    float64       // Höchster gemessener Wert
	AvgValue    float64       // Durchschnittswert
	Duration    time.Duration // Wie lange das Problem bestand
}

type PerfThresholds struct {
	CPUCritical      float64 // z.B. 90%
	CPUWarning       float64 // z.B. 75%
	RAMCritical      float64 // z.B. 90%
	RAMWarning       float64 // z.B. 75%
	DiskBusyWarning  float64 // z.B. 80%
	DiskBusyCritical float64 // z.B. 95%
	NetworkWarning   float64 // z.B. 80% der Link-Speed
	NetworkCritical  float64 // z.B. 95% der Link-Speed
}

// Erweitere deine bestehende PerformanceAnalyzer struct um diese Analyse-Methoden:

func (pa *PerformanceAnalyzer) AnalyzePerformance() []PerformanceIssue {
	pa.mu.RLock()
	defer pa.mu.RUnlock()

	if len(pa.stats) <= 1 {
		return nil
	}

	// Standard-Thresholds definieren
	thresholds := PerfThresholds{
		CPUCritical:      90.0,
		CPUWarning:       75.0,
		RAMCritical:      90.0,
		RAMWarning:       75.0,
		DiskBusyWarning:  80.0,
		DiskBusyCritical: 95.0,
		NetworkWarning:   80.0,
		NetworkCritical:  95.0,
	}

	var issues []PerformanceIssue

	// Verschiedene Komponenten analysieren
	if pa.monitorCPU {
		issues = append(issues, pa.analyzeCPU(thresholds)...)
	}
	if pa.monitorRAM {
		issues = append(issues, pa.analyzeRAM(thresholds)...)
	}
	issues = append(issues, pa.analyzeNetwork(thresholds)...)
	issues = append(issues, pa.analyzeDisk(thresholds)...)

	// Nach Severity sortieren
	sort.Slice(issues, func(i, j int) bool {
		severityOrder := map[string]int{"Critical": 0, "Warning": 1, "Info": 2}
		return severityOrder[issues[i].Severity] < severityOrder[issues[j].Severity]
	})

	return issues
}

func (pa *PerformanceAnalyzer) analyzeCPU(thresholds PerfThresholds) []PerformanceIssue {
	var issues []PerformanceIssue
	var cpuValues []float64
	var highLoadDuration time.Duration
	var criticalCount, warningCount int

	for i, stat := range pa.stats[1:] {
		cpuValues = append(cpuValues, stat.CPUPercent)

		if stat.CPUPercent >= thresholds.CPUCritical {
			criticalCount++
			if i > 0 {
				highLoadDuration += stat.Timestamp.Sub(pa.stats[i].Timestamp)
			}
		} else if stat.CPUPercent >= thresholds.CPUWarning {
			warningCount++
		}
	}

	if len(cpuValues) == 0 {
		return issues
	}

	avgCPU := average(cpuValues)
	maxCPU := maximum(cpuValues)

	if criticalCount > 0 {
		issues = append(issues, PerformanceIssue{
			Component:   "CPU",
			Device:      "",
			Severity:    "Critical",
			Description: fmt.Sprintf("CPU-Auslastung kritisch hoch (%.1f%% durchschnittlich, %.1f%% maximum)", avgCPU, maxCPU),
			MaxValue:    maxCPU,
			AvgValue:    avgCPU,
			Duration:    highLoadDuration,
		})
	} else if warningCount > len(cpuValues)/2 { // Mehr als 50% der Zeit über Warning-Threshold
		issues = append(issues, PerformanceIssue{
			Component:   "CPU",
			Device:      "",
			Severity:    "Warning",
			Description: fmt.Sprintf("CPU-Auslastung häufig erhöht (%.1f%% durchschnittlich)", avgCPU),
			MaxValue:    maxCPU,
			AvgValue:    avgCPU,
		})
	}

	return issues
}

func (pa *PerformanceAnalyzer) analyzeRAM(thresholds PerfThresholds) []PerformanceIssue {
	var issues []PerformanceIssue

	totalRAM := getTotalRAM()
	if totalRAM == 0 {
		return issues
	}

	var ramValues []float64
	for _, stat := range pa.stats[1:] {
		ramValues = append(ramValues, stat.MemUsedGB)
	}

	if len(ramValues) == 0 {
		return issues
	}

	avgRAM := average(ramValues)
	maxRAM := maximum(ramValues)
	avgPercent := (avgRAM / totalRAM) * 100
	maxPercent := (maxRAM / totalRAM) * 100

	if maxPercent >= thresholds.RAMCritical {
		issues = append(issues, PerformanceIssue{
			Component:   "RAM",
			Device:      "",
			Severity:    "Critical",
			Description: fmt.Sprintf("Speicher kritisch knapp (%.1f%% durchschnittlich, %.1f%% maximum)", avgPercent, maxPercent),
			MaxValue:    maxRAM,
			AvgValue:    avgRAM,
		})
	} else if avgPercent >= thresholds.RAMWarning {
		issues = append(issues, PerformanceIssue{
			Component:   "RAM",
			Device:      "",
			Severity:    "Warning",
			Description: fmt.Sprintf("Speicherverbrauch erhöht (%.1f%% durchschnittlich)", avgPercent),
			MaxValue:    maxRAM,
			AvgValue:    avgRAM,
		})
	}

	return issues
}

func (pa *PerformanceAnalyzer) analyzeNetwork(thresholds PerfThresholds) []PerformanceIssue {
	var issues []PerformanceIssue

	for _, dev := range pa.networkDevs {
		speed := getNetSpeed(dev)
		if speed <= 0 {
			continue
		}

		var rxValues, txValues []float64
		var errorCount int64

		for _, stat := range pa.stats[1:] {
			if netStat, exists := stat.Network[dev]; exists {
				rxValues = append(rxValues, netStat.RxMB*8) // MB/s zu Mbit/s
				txValues = append(txValues, netStat.TxMB*8)
				errorCount += netStat.Errors
			}
		}

		if len(rxValues) == 0 {
			continue
		}

		maxRx := maximum(rxValues)
		maxTx := maximum(txValues)
		avgRx := average(rxValues)
		avgTx := average(txValues)

		maxRxPercent := (maxRx / float64(speed)) * 100
		maxTxPercent := (maxTx / float64(speed)) * 100
		avgRxPercent := (avgRx / float64(speed)) * 100
		avgTxPercent := (avgTx / float64(speed)) * 100

		// RX-Analyse
		if maxRxPercent >= thresholds.NetworkCritical {
			issues = append(issues, PerformanceIssue{
				Component:   "Network",
				Device:      dev,
				Severity:    "Critical",
				Description: fmt.Sprintf("Download-Bandbreite am Limit (%.1f%% von %d Mbit/s)", maxRxPercent, speed),
				MaxValue:    maxRx,
				AvgValue:    avgRx,
			})
		} else if avgRxPercent >= thresholds.NetworkWarning {
			issues = append(issues, PerformanceIssue{
				Component:   "Network",
				Device:      dev,
				Severity:    "Warning",
				Description: fmt.Sprintf("Download-Bandbreite häufig hoch (%.1f%% durchschnittlich)", avgRxPercent),
				MaxValue:    maxRx,
				AvgValue:    avgRx,
			})
		}

		// TX-Analyse
		if maxTxPercent >= thresholds.NetworkCritical {
			issues = append(issues, PerformanceIssue{
				Component:   "Network",
				Device:      dev,
				Severity:    "Critical",
				Description: fmt.Sprintf("Upload-Bandbreite am Limit (%.1f%% von %d Mbit/s)", maxTxPercent, speed),
				MaxValue:    maxTx,
				AvgValue:    avgTx,
			})
		} else if avgTxPercent >= thresholds.NetworkWarning {
			issues = append(issues, PerformanceIssue{
				Component:   "Network",
				Device:      dev,
				Severity:    "Warning",
				Description: fmt.Sprintf("Upload-Bandbreite häufig hoch (%.1f%% durchschnittlich)", avgTxPercent),
				MaxValue:    maxTx,
				AvgValue:    avgTx,
			})
		}

		// Fehler-Analyse
		if errorCount > 0 {
			issues = append(issues, PerformanceIssue{
				Component:   "Network",
				Device:      dev,
				Severity:    "Warning",
				Description: fmt.Sprintf("Netzwerkfehler aufgetreten (%d Fehler)", errorCount),
				MaxValue:    float64(errorCount),
			})
		}
	}

	return issues
}

func (pa *PerformanceAnalyzer) analyzeDisk(thresholds PerfThresholds) []PerformanceIssue {
	var issues []PerformanceIssue

	for _, dev := range pa.diskDevs {
		var busyValues, readValues, writeValues []float64
		var highBusyDuration time.Duration
		var criticalBusyCount int

		for i, stat := range pa.stats[1:] {
			if diskStat, exists := stat.Disks[dev]; exists {
				busyValues = append(busyValues, diskStat.BusyPct)
				readValues = append(readValues, diskStat.ReadMB)
				writeValues = append(writeValues, diskStat.WriteMB)

				if diskStat.BusyPct >= thresholds.DiskBusyCritical {
					criticalBusyCount++
					if i > 0 {
						highBusyDuration += stat.Timestamp.Sub(pa.stats[i].Timestamp)
					}
				}
			}
		}

		if len(busyValues) == 0 {
			continue
		}

		avgBusy := average(busyValues)
		maxBusy := maximum(busyValues)
		maxRead := maximum(readValues)
		maxWrite := maximum(writeValues)

		if maxBusy >= thresholds.DiskBusyCritical {
			issues = append(issues, PerformanceIssue{
				Component:   "Disk",
				Device:      dev,
				Severity:    "Critical",
				Description: fmt.Sprintf("Festplatte überlastet (%.1f%% Busy durchschnittlich, %.1f%% maximum)", avgBusy, maxBusy),
				MaxValue:    maxBusy,
				AvgValue:    avgBusy,
				Duration:    highBusyDuration,
			})
		} else if avgBusy >= thresholds.DiskBusyWarning {
			issues = append(issues, PerformanceIssue{
				Component:   "Disk",
				Device:      dev,
				Severity:    "Warning",
				Description: fmt.Sprintf("Festplatte häufig ausgelastet (%.1f%% Busy durchschnittlich)", avgBusy),
				MaxValue:    maxBusy,
				AvgValue:    avgBusy,
			})
		}

		// Zusätzliche Analyse für sehr hohe I/O-Werte
		if maxRead > 100.0 || maxWrite > 100.0 {
			issues = append(issues, PerformanceIssue{
				Component:   "Disk",
				Device:      dev,
				Severity:    "Info",
				Description: fmt.Sprintf("Hohe Festplatten-I/O (max %.1f MB/s Read, %.1f MB/s Write)", maxRead, maxWrite),
				MaxValue:    math.Max(maxRead, maxWrite),
			})
		}
	}

	return issues
}

// PrintAnalysis gibt die Performance-Analyse aus
func (pa *PerformanceAnalyzer) PrintAnalysis() {
	issues := pa.AnalyzePerformance()

	if len(issues) == 0 {
		fmt.Println("\n=== PERFORMANCE-ANALYSE ===")
		fmt.Println("✅ Keine Performance-Probleme erkannt.")
		fmt.Println("   Alle überwachten Komponenten arbeiten im normalen Bereich.")
		return
	}

	fmt.Println("\n=== PERFORMANCE-ANALYSE ===")

	criticalCount := 0
	warningCount := 0

	for _, issue := range issues {
		var icon string
		switch issue.Severity {
		case "Critical":
			icon = "🔴"
			criticalCount++
		case "Warning":
			icon = "🟡"
			warningCount++
		case "Info":
			icon = "ℹ️"
		}

		fmt.Printf("%s %s", icon, issue.Description)
		if issue.Device != "" {
			fmt.Printf(" [%s]", issue.Device)
		}

		if issue.Duration > 0 {
			fmt.Printf(" (Dauer: %v)", issue.Duration.Round(time.Second))
		}
		fmt.Println()
	}

	// Zusammenfassung
	fmt.Println()
	if criticalCount > 0 {
		fmt.Printf("⚠️  %d kritische Problem(e) erkannt - System möglicherweise überlastet!\n", criticalCount)
	}
	if warningCount > 0 {
		fmt.Printf("⚡ %d Warnung(en) - Performance könnte verbessert werden.\n", warningCount)
	}

	// Empfehlungen
	pa.printRecommendations(issues)
}

func (pa *PerformanceAnalyzer) printRecommendations(issues []PerformanceIssue) {
	if len(issues) == 0 {
		return
	}

	fmt.Println("\n=== EMPFEHLUNGEN ===")

	hasCPUProblem := false
	hasRAMProblem := false
	hasDiskProblem := false
	hasNetworkProblem := false

	for _, issue := range issues {
		switch issue.Component {
		case "CPU":
			if issue.Severity == "Critical" && !hasCPUProblem {
				fmt.Println("🔧 CPU: Rechenintensive Prozesse identifizieren und optimieren")
				hasCPUProblem = true
			}
		case "RAM":
			if issue.Severity == "Critical" && !hasRAMProblem {
				fmt.Println("🔧 RAM: Speicher-intensive Anwendungen schließen oder RAM erweitern")
				hasRAMProblem = true
			}
		case "Disk":
			if issue.Severity == "Critical" && !hasDiskProblem {
				fmt.Println("🔧 Disk: I/O-Last reduzieren, SSD verwenden oder RAID-Setup optimieren")
				hasDiskProblem = true
			}
		case "Network":
			if issue.Severity == "Critical" && !hasNetworkProblem {
				fmt.Println("🔧 Network: Bandbreite erhöhen oder Datenübertragung optimieren")
				hasNetworkProblem = true
			}
		}
	}

	// Allgemeine Empfehlung wenn keine kritischen Probleme auf diesem System
	if countBySeverity(issues, "Critical") == 0 {
		fmt.Println("💡 Mögliche externe Faktoren:")
		fmt.Println("   - Netzwerk-Latenz oder Bandbreitenbegrenzung")
		fmt.Println("   - Langsame Quell-/Zielsysteme")
		fmt.Println("   - Anwendungslogik oder Dateisystem-Konfiguration")
	}
}

// Hilfsfunktionen
func getTotalRAM() float64 {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "MemTotal:") {
			var memTotal uint64
			fmt.Sscanf(line, "MemTotal: %d kB", &memTotal)
			return float64(memTotal) / (1024 * 1024) // GB
		}
	}
	return 0
}

func average(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

func maximum(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	max := values[0]
	for _, v := range values {
		if v > max {
			max = v
		}
	}
	return max
}

func countBySeverity(issues []PerformanceIssue, severity string) int {
	count := 0
	for _, issue := range issues {
		if issue.Severity == severity {
			count++
		}
	}
	return count
}
