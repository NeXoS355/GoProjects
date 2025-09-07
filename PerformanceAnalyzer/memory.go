package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// getMemStats liest RAMStatistiken aus /proc/meminfo
func (pa *PerformanceAnalyzer) getMemStats() (MemStats, error) {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return MemStats{}, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var memTotal, memFree, dirty, wb uint64
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "MemTotal:") {
			fmt.Sscanf(line, "MemTotal: %d kB", &memTotal)
		} else if strings.HasPrefix(line, "MemAvailable:") {
			fmt.Sscanf(line, "MemAvailable: %d kB", &memFree)
		} else if strings.HasPrefix(line, "Dirty:") {
			fmt.Sscanf(line, "Dirty: %d kB", &dirty)
		} else if strings.HasPrefix(line, "Writeback:") {
			fmt.Sscanf(line, "Writeback: %d kB", &wb)
		}
	}
	usedGB := float64((memTotal - memFree)) / float64((1024 * 1024))
	usedPerct := float64(memTotal-memFree) * 100.0 / float64(memTotal)
	dirtyMB := float64(dirty) / 1024.0
	wbMB := float64(wb) / 1024.0
	return MemStats{
		usedGB:    usedGB,
		usedPerct: usedPerct,
		dirty:     dirtyMB,
		writeback: wbMB,
	}, nil
}
