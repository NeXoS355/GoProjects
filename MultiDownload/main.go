// main.go
package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
)

// StartDownloads - Ruft die mDownload Funktion mit den konfigurierten Parametern auf
func StartDownloads(config *DownloadConfig) {

	// Baue mDownload Command zusammen
	args := []string{}

	// Worker (-w)
	args = append(args, "-w", strconv.Itoa(config.Workers))

	// Bandwidth Limit (-l) - nur wenn gesetzt
	if config.LimitKB > 0 {
		args = append(args, "-l", strconv.Itoa(config.LimitKB))
	}

	// Output Directory (-o)
	args = append(args, "-o", config.OutputDir)

	for i, url := range config.URLs {
		if len(config.Names[i]) > 0 {
			args = append(args, fmt.Sprintf("%s@%s", url, config.Names[i]))
		} else {
			args = append(args, url)
		}
	}

	// Command ausführen
	//fmt.Printf("\nFühre aus: /usr/local/bin/mDownload %s\n", fmt.Sprintf("%v", args))
	//fmt.Println("=" + strings.Repeat("=", 50))

	cmd := exec.Command("/usr/local/bin/mDownload", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		fmt.Printf("Fehler beim Ausführen von mDownload: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("\nDownloads abgeschlossen!")
}

func main() {
	// TUI anzeigen
	config, shouldStart := ShowTUI()

	if !shouldStart {
		fmt.Println("Download abgebrochen.")
		os.Exit(0)
	} else {
		// Downloads starten
		StartDownloads(config)
	}

}
