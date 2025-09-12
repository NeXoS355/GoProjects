package main

import (
	"fmt"
	"sort"
	"time"
)

func NewPerformanceAnalyzer(interval time.Duration, netDevs, diskDevs []string, monitorCPU, monitorRAM bool) *PerformanceAnalyzer {
	return &PerformanceAnalyzer{
		stats:       make([]Stats, 0),
		interval:    interval,
		networkDevs: netDevs,
		diskDevs:    diskDevs,
		monitorCPU:  monitorCPU,
		monitorRAM:  monitorRAM,
		prevNet:     make(map[string]NetworkStats),
		prevDisk:    make(map[string]DiskStats),
	}
}

// collectStats sammelt alle Systemstatistiken
func (pa *PerformanceAnalyzer) collectStats() Stats {
	stat := Stats{
		Timestamp: time.Now(),
		Network:   make(map[string]NetStatsPerDev),
		Disks:     make(map[string]DiskStatsPerDev),
	}

	// Netzwerk nur wenn Devices vorhanden
	for _, dev := range pa.networkDevs {
		netStats, err := pa.getNetworkStats(dev) // liefert NetworkStats
		if err == nil {
			if pa.initialized {
				timeDiff := pa.interval.Seconds()
				prevNetStats := pa.prevNet[dev] // ← Typ NetworkStats
				stat.Network[dev] = NetStatsPerDev{
					RxMB:   float64(netStats.RxBytes-prevNetStats.RxBytes) / (1024 * 1024) / timeDiff,
					TxMB:   float64(netStats.TxBytes-prevNetStats.TxBytes) / (1024 * 1024) / timeDiff,
					Errors: netStats.Errors - prevNetStats.Errors,
				}
			}
			pa.prevNet[dev] = netStats
		}
	}

	// Disks nur wenn Devices vorhanden
	for _, dev := range pa.diskDevs {
		diskStats, err := pa.getDiskStats(dev)
		if err != nil || len(dev) == 0 {
			continue
		}

		prev, hasPrev := pa.prevDisk[dev]

		if pa.initialized && hasPrev {
			timeDiff := pa.interval.Seconds()
			stat.Disks[dev] = DiskStatsPerDev{
				ReadMB:  float64(diskStats.ReadBytes-prev.ReadBytes) / (1024 * 1024) / timeDiff,
				WriteMB: float64(diskStats.WriteBytes-prev.WriteBytes) / (1024 * 1024) / timeDiff,
				BusyPct: float64(diskStats.BusyMillis-prev.BusyMillis) / (timeDiff * 1000) * 100.0,
			}
		} else {
			// Erstes Intervall: nur prev setzen, noch keine MB/s berechnen
			stat.Disks[dev] = DiskStatsPerDev{
				ReadMB:  0,
				WriteMB: 0,
				BusyPct: 0,
			}
		}

		// prevDisk immer aktualisieren
		pa.prevDisk[dev] = diskStats
	}

	// CPU und RAM nur wenn aktiviert
	if pa.monitorCPU {
		stat.CPUPercent = pa.getCPUUsageAll()
	}

	if pa.monitorRAM {
		memStats, err := pa.getMemStats()
		if err == nil {
			if pa.memUsedAtStart == 0 {
				pa.memUsedAtStart = memStats.usedGB
			}
			stat.MemUsedGB = memStats.usedGB
			stat.MemUsedPerct = memStats.usedPerct
			stat.MemDirty = memStats.dirty
			stat.MemWritebac = memStats.writeback
		}
	}

	pa.initialized = true
	return stat
}

func (pa *PerformanceAnalyzer) addStat(stat Stats) {
	pa.mu.Lock()
	defer pa.mu.Unlock()
	pa.stats = append(pa.stats, stat)
}

// printCurrentStats zeigt die aktuellen Statistiken an
func (pa *PerformanceAnalyzer) printCurrentStats(stat Stats) {
	if !pa.initialized {
		fmt.Println("Initialisiere Messungen...")
		return
	}

	// Cursor nach oben bewegen und Zeilen löschen
	// (Anzahl der Zeilen anpassen, die du neu zeichnen willst)
	fmt.Print("\033[H\033[J") // Clear screen + Cursor Home

	fmt.Printf("[%s]\n", stat.Timestamp.Format("15:04:05.000"))

	if len(stat.Network) > 0 {
		for dev, v := range stat.Network {
			speed := getNetSpeed(dev) // MBit/s
			if speed > 0 {
				rxMbit := v.RxMB * 8 // MB/s → Mbit/s
				txMbit := v.TxMB * 8
				rxPct := (rxMbit / float64(speed)) * 100
				txPct := (txMbit / float64(speed)) * 100

				fmt.Printf("Net:  %-12s: ↓%.2f MB/s (%.1f%%) | ↑%.2f MB/s (%.1f%%) | Speed: %d Mbit/s | Err: %.1f%%\n",
					dev, v.RxMB, rxPct, v.TxMB, txPct, speed, v.Errors)
			} else {
				fmt.Printf("Net:  %-12s]: ↓%.2f / ↑%.2f MB/s (Speed n/a) | Err: %.1f%%\n", dev, v.RxMB, v.TxMB, v.Errors)
			}
		}
	}

	if len(stat.Disks) > 0 {
		var devs []string
		for dev := range stat.Disks {
			devs = append(devs, dev)
		}
		sort.Strings(devs)
		for _, dev := range devs {
			v := stat.Disks[dev]
			fmt.Printf("Disk: %-12s: R%.2f | W%.2f MB/s | Busy %.1f%%\n", dev, v.ReadMB, v.WriteMB, v.BusyPct)
		}
	}

	if pa.monitorCPU {
		warning := false
		var warningCores []int
		for i, corePercent := range stat.CPUPercent {
			if corePercent > 75 {
				warning = true
				warningCores = append(warningCores, i)
			}
		}
		if warning {
			fmt.Printf("CPU:              : %.1f%%", stat.CPUPercent[0])
			if len(warningCores) > 0 {
				for i := range warningCores {
					fmt.Printf(" | !Kern%d!", warningCores[i])
				}
			} else {
				fmt.Printf("\n")
			}
		} else {
			fmt.Printf("CPU:              : %.1f%%\n", stat.CPUPercent[0])
		}
	}

	diff := stat.MemUsedGB - pa.memUsedAtStart
	if pa.monitorRAM {
		fmt.Printf("RAM:              : %.2f GB (%.1f%%) %+.2f GB | Dirty: %.2f MiB | Writeback: %.2f MiB\n", stat.MemUsedGB, stat.MemUsedPerct, diff, stat.MemDirty, stat.MemWritebac)
	}
}
