package main

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

// getCPUUsageAll liefert Nutzung aller CPUs (Gesamt + Kerne)
func (pa *PerformanceAnalyzer) getCPUUsageAll() []float64 {
	file, err := os.Open("/proc/stat")
	if err != nil {
		return nil
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	usages := []float64{}
	var newIdle, newTotal []uint64

	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 0 || !strings.HasPrefix(fields[0], "cpu") {
			break // nur CPU-Zeilen interessieren
		}

		// Zahlen parsen
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

		newIdle = append(newIdle, idle)
		newTotal = append(newTotal, total)

		if pa.initialized {
			idx := len(newIdle) - 1
			idleDiff := idle - pa.prevIdle[idx]
			totalDiff := total - pa.prevTotal[idx]
			usage := 100.0 * (1.0 - float64(idleDiff)/float64(totalDiff))
			usages = append(usages, usage)
		} else {
			usages = append(usages, 0.0)
		}
	}

	pa.prevIdle = newIdle
	pa.prevTotal = newTotal
	pa.initialized = true
	return usages
}
