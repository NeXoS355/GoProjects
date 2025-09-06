package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Stats struct {
	Timestamp  time.Time
	CPUPercent float64
	MemUsedGB  float64

	Network map[string]NetStatsPerDev
	Disks   map[string]DiskStatsPerDev
}

type PerformanceAnalyzer struct {
	stats    []Stats
	interval time.Duration
	running  bool
	mu       sync.RWMutex

	networkDevs []string
	diskDevs    []string
	monitorCPU  bool
	monitorRAM  bool

	prevNet  map[string]NetworkStats
	prevDisk map[string]DiskStats

	initialized bool
	prevIdle    uint64
	prevTotal   uint64
}

type NetStatsPerDev struct {
	RxMB   float64
	TxMB   float64
	Errors int64
}

type DiskStatsPerDev struct {
	ReadMB  float64
	WriteMB float64
	BusyPct float64
}

type NetworkStats struct {
	RxBytes uint64
	TxBytes uint64
	Errors  uint64
}

type DiskStats struct {
	ReadBytes  uint64
	WriteBytes uint64
	BusyMillis uint64
}

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
}

func (pa *PerformanceAnalyzer) isRunning() bool {
	pa.mu.RLock()
	defer pa.mu.RUnlock()
	return pa.running
}

func (pa *PerformanceAnalyzer) addStat(stat Stats) {
	pa.mu.Lock()
	defer pa.mu.Unlock()
	pa.stats = append(pa.stats, stat)
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
					Errors: int64(netStats.Errors - prevNetStats.Errors),
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
		stat.CPUPercent = pa.getCPUUsage()
	}
	if pa.monitorRAM {
		stat.MemUsedGB = pa.getMemoryUsage()
	}

	pa.initialized = true
	return stat
}

// getNetworkStats liest Netzwerkstatistiken aus /proc/net/dev
func (pa *PerformanceAnalyzer) getNetworkStats(dev string) (NetworkStats, error) {
	file, err := os.Open("/proc/net/dev")
	if err != nil {
		return NetworkStats{}, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, dev+":") {
			fields := strings.Fields(line)
			if len(fields) >= 11 {
				rxBytes, _ := strconv.ParseUint(fields[1], 10, 64)
				txBytes, _ := strconv.ParseUint(fields[9], 10, 64)
				errors, _ := strconv.ParseUint(fields[2], 10, 64)
				return NetworkStats{
					RxBytes: rxBytes,
					TxBytes: txBytes,
					Errors:  errors,
				}, nil
			}
		}
	}
	return NetworkStats{}, fmt.Errorf("device %s not found", dev)
}

// getNetSpeed liest die Interface-Geschwindigkeit aus /sys/class/net/<dev>/speed
func getNetSpeed(dev string) int {
	data, err := os.ReadFile(fmt.Sprintf("/sys/class/net/%s/speed", dev))
	if err != nil {
		return -1
	}
	speedStr := strings.TrimSpace(string(data))
	speed, err := strconv.Atoi(speedStr)
	if err != nil {
		return -1
	}
	return speed // MBit/s
}

// getDiskStats liest Festplattenstatistiken aus /proc/diskstats
func (pa *PerformanceAnalyzer) getDiskStats(dev string) (DiskStats, error) {
	file, err := os.Open("/proc/diskstats")
	if err != nil {
		return DiskStats{}, err
	}
	defer file.Close()

	sectorSize := getSectorSize(dev)

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 14 && fields[2] == dev {
			readSectors, _ := strconv.ParseUint(fields[5], 10, 64)  // sectors read
			writeSectors, _ := strconv.ParseUint(fields[9], 10, 64) // sectors written
			busyTime, _ := strconv.ParseUint(fields[12], 10, 64)    // busy Zeit in ms
			return DiskStats{
				ReadBytes:  readSectors * sectorSize,
				WriteBytes: writeSectors * sectorSize,
				BusyMillis: busyTime,
			}, nil
		}
	}
	return DiskStats{}, fmt.Errorf("device %s not found", dev)
}

func getSectorSize(dev string) uint64 {
	data, err := os.ReadFile(fmt.Sprintf("/sys/block/%s/queue/logical_block_size", dev))
	if err != nil {
		return 512 // fallback
	}
	size, _ := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)
	return size
}

// getCPUUsage gibt eine CPU-Auslastung zurück
func (pa *PerformanceAnalyzer) getCPUUsage() float64 {
	file, _ := os.Open("/proc/stat")
	defer file.Close()
	scanner := bufio.NewScanner(file)
	if scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if fields[0] == "cpu" {
			var vals []uint64
			for _, f := range fields[1:] {
				v, _ := strconv.ParseUint(f, 10, 64)
				vals = append(vals, v)
			}
			idle := vals[3]
			total := uint64(0)
			for _, v := range vals {
				total += v
			}
			if pa.initialized {
				idleDiff := idle - pa.prevIdle
				totalDiff := total - pa.prevTotal
				usage := 100.0 * (1.0 - float64(idleDiff)/float64(totalDiff))
				pa.prevIdle = idle
				pa.prevTotal = total
				return usage
			}
			pa.prevIdle = idle
			pa.prevTotal = total
		}
	}
	return 0.0
}

// getMemoryUsage gibt die RAM-Auslastung in GB zurück
func (pa *PerformanceAnalyzer) getMemoryUsage() float64 {
	file, _ := os.Open("/proc/meminfo")
	defer file.Close()
	scanner := bufio.NewScanner(file)
	var memTotal, memFree uint64
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "MemTotal:") {
			fmt.Sscanf(line, "MemTotal: %d kB", &memTotal)
		} else if strings.HasPrefix(line, "MemAvailable:") {
			fmt.Sscanf(line, "MemAvailable: %d kB", &memFree)
		}
	}
	used := memTotal - memFree
	return float64(used) / (1024 * 1024) // GB
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

	fmt.Printf("[%s]\n", stat.Timestamp.Format("15:04:05"))

	if len(stat.Network) > 0 {
		for dev, v := range stat.Network {
			speed := getNetSpeed(dev) // MBit/s
			if speed > 0 {
				rxMbit := v.RxMB * 8 // MB/s → Mbit/s
				txMbit := v.TxMB * 8
				rxPct := (rxMbit / float64(speed)) * 100
				txPct := (txMbit / float64(speed)) * 100

				fmt.Printf("Net:  %-12s: ↓%.2f MB/s (%.1f%%) | ↑%.2f MB/s (%.1f%%) | Speed: %d Mbit/s\n",
					dev, v.RxMB, rxPct, v.TxMB, txPct, speed)
			} else {
				fmt.Printf("Net:  %-12s]: ↓%.2f / ↑%.2f MB/s (Speed n/a)\n", dev, v.RxMB, v.TxMB)
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
		fmt.Printf("CPU:              : %.1f%%\n", stat.CPUPercent)
	}
	if pa.monitorRAM {
		fmt.Printf("RAM:              : %.2f GB\n", stat.MemUsedGB)
	}
}

func (pa *PerformanceAnalyzer) PrintSummary() {
	pa.mu.RLock()
	defer pa.mu.RUnlock()

	if len(pa.stats) == 0 {
		fmt.Println("Keine Daten gesammelt.")
		return
	}

	fmt.Println("\n\n=== PERFORMANCE ZUSAMMENFASSUNG ===")

	// Netzwerk: pro Device Stats
	netAvgRx := make(map[string]float64)
	netAvgTx := make(map[string]float64)
	netMaxRx := make(map[string]float64)
	netMaxTx := make(map[string]float64)
	netErrors := make(map[string]int64)
	netCount := make(map[string]float64)

	// Disk: pro Device Stats
	diskAvgR := make(map[string]float64)
	diskAvgW := make(map[string]float64)
	diskMaxR := make(map[string]float64)
	diskMaxW := make(map[string]float64)
	diskAvgBusy := make(map[string]float64)
	diskMaxBusy := make(map[string]float64)
	diskCount := make(map[string]float64)

	var avgCPU, maxCPU float64
	var cpuCount float64

	// Durch alle gesammelten Stats iterieren (außer dem ersten, da kein Delta)
	for _, stat := range pa.stats[1:] {
		// Netzwerk
		for dev, ns := range stat.Network {
			if ns.RxMB+ns.TxMB > 0.1 { // ignoriert idle times
				netAvgRx[dev] += ns.RxMB
				netAvgTx[dev] += ns.TxMB
				netErrors[dev] += ns.Errors
				netCount[dev]++
				if ns.RxMB > netMaxRx[dev] {
					netMaxRx[dev] = ns.RxMB
				}
				if ns.TxMB > netMaxTx[dev] {
					netMaxTx[dev] = ns.TxMB
				}
			}
		}

		// Disks
		for dev, ds := range stat.Disks {
			if ds.ReadMB+ds.WriteMB > 1.0 { // ignoriert idle times
				diskAvgR[dev] += ds.ReadMB
				diskAvgW[dev] += ds.WriteMB
				diskAvgBusy[dev] += ds.BusyPct
				diskCount[dev]++

				if ds.ReadMB > diskMaxR[dev] {
					diskMaxR[dev] = ds.ReadMB
				}
				if ds.WriteMB > diskMaxW[dev] {
					diskMaxW[dev] = ds.WriteMB
				}
				if ds.BusyPct > diskMaxBusy[dev] {
					diskMaxBusy[dev] = ds.BusyPct
				}
			}
		}

		// CPU
		if pa.monitorCPU {
			avgCPU += stat.CPUPercent
			if stat.CPUPercent > maxCPU {
				maxCPU = stat.CPUPercent
			}
			cpuCount++
		}
	}

	// Netzwerk-Ausgabe
	if len(netCount) > 0 {
		fmt.Println("Netzwerk:")
		for dev := range netCount {
			fmt.Printf("  [%s] ↓ Ø%.2f (Max %.2f) MiB/s | ↑ Ø%.2f (Max %.2f) MiB/s | Errors: %d\n",
				dev,
				netAvgRx[dev]/netCount[dev], netMaxRx[dev],
				netAvgTx[dev]/netCount[dev], netMaxTx[dev],
				netErrors[dev],
			)
		}
	}

	// Disk-Ausgabe inkl. Busy
	if len(diskCount) > 0 {
		fmt.Println("\nFestplatten:")
		for dev := range diskCount {
			fmt.Printf(
				"  [%s] Read Ø%.2f (Max %.2f) MiB/s | Write Ø%.2f (Max %.2f) MiB/s | Busy Ø%.1f%% (Max %.1f%%)\n",
				dev,
				diskAvgR[dev]/diskCount[dev], diskMaxR[dev],
				diskAvgW[dev]/diskCount[dev], diskMaxW[dev],
				diskAvgBusy[dev]/diskCount[dev], diskMaxBusy[dev],
			)
		}
	}

	// CPU
	if cpuCount > 0 {
		fmt.Println("\nSystem:")
		fmt.Printf("  CPU: Ø%.1f%% (Max %.1f%%)\n", avgCPU/cpuCount, maxCPU)
	}

	// Dauer
	duration := pa.stats[len(pa.stats)-1].Timestamp.Sub(pa.stats[0].Timestamp)
	fmt.Printf("\nGesamte Überwachungsdauer: %v\n", duration.Round(time.Second))
}

type multiFlag []string

func (m *multiFlag) String() string       { return strings.Join(*m, ",") }
func (m *multiFlag) Set(val string) error { *m = append(*m, val); return nil }

var (
	intervalSec int
	cmdStr      string
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
	flag.IntVar(&intervalSec, "i", 1, "sampling rate")
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
		fmt.Println("  -i            change sampling rate (default:1)")
		fmt.Println("  -cmd          execute command while monitoring with perfAnalyzer")
		fmt.Println("")
		fmt.Println("Example:")
		fmt.Println("  perfAnalyzer -n enp1s0 -d sda1 -c -r -i 2 -cmd 'cp /some/random/file /to/some/random/path'")
		return
	}

	fmt.Printf("Used Network Interface: %s\n", netDevs)
	fmt.Printf("Use Disk: %s\n", diskDevs)
	fmt.Println()

	interval := time.Duration(intervalSec) * time.Second
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
