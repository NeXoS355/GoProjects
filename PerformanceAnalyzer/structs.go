package main

import (
	"sync"
	"time"
)

type Stats struct {
	Timestamp    time.Time
	CPUPercent   []float64
	MemUsedGB    float64
	MemUsedPerct float64
	MemDirty     float64
	MemWritebac  float64

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

	initialized    bool
	prevIdle       []uint64
	prevTotal      []uint64
	memUsedAtStart float64
}

type NetStatsPerDev struct {
	RxMB   float64
	TxMB   float64
	Errors float64
}

type DiskStatsPerDev struct {
	ReadMB  float64
	WriteMB float64
	BusyPct float64
}

type NetworkStats struct {
	RxBytes uint64
	TxBytes uint64
	Errors  float64
}

type DiskStats struct {
	ReadBytes  uint64
	WriteBytes uint64
	BusyMillis uint64
}

type MemStats struct {
	usedGB    float64
	usedPerct float64
	dirty     float64
	writeback float64
}
