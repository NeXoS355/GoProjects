package main

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

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
