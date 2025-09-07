package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

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
