package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
)

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
				rxBytes, _ := strconv.ParseUint(fields[1], 10, 64) // recieved Bytes
				txBytes, _ := strconv.ParseUint(fields[9], 10, 64) // transmitted Bytes

				rxPackets, _ := strconv.ParseUint(fields[2], 10, 64) // recieved Packets
				rxErrors, _ := strconv.ParseUint(fields[3], 10, 64)  // revieved Errors
				rxDropped, _ := strconv.ParseUint(fields[4], 10, 64) // revieved Dropped

				txPackets, _ := strconv.ParseUint(fields[10], 10, 64) // transmitted Packets
				txErrors, _ := strconv.ParseUint(fields[11], 10, 64)  // transmitted Errors
				txDropped, _ := strconv.ParseUint(fields[12], 10, 64) // transmitted Dropped

				var rxErrorRate, txErrorRate float64

				if rxPackets > 0 {
					rxErrorRate = float64(rxErrors+rxDropped) / float64(rxPackets) * 100.0
				}
				if txPackets > 0 {
					txErrorRate = float64(txErrors+txDropped) / float64(txPackets) * 100.0
				}

				errorRate := (rxErrorRate + txErrorRate) / 2.0

				return NetworkStats{
					RxBytes: rxBytes,
					TxBytes: txBytes,
					Errors:  errorRate,
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

func validateNetworkInterface(name string) error {
	ifaces, err := net.Interfaces()
	if err != nil {
		return fmt.Errorf("could not check Network Devices: %v", err)
	}

	for _, iface := range ifaces {
		if iface.Name == name {
			return nil // alles OK
		}
	}
	var netDevs []string
	for _, i := range ifaces {
		netDevs = append(netDevs, i.Name)
	}
	return fmt.Errorf("Network-Device '%s' not found.\nAvailable Devices are:\n - %s", name, strings.Join(netDevs, "\n - "))
}
