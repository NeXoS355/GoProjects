// Update Status Function// ui.go
package main

import (
	"fmt"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// DownloadConfig definiert die Konfiguration für Downloads
type DownloadConfig struct {
	Workers   int
	LimitKB   int
	OutputDir string
	URLs      []string
	Names     []string // Custom Namen für URLs
}

func detectFilename(url string) string {
	client := &http.Client{Timeout: 5 * time.Second}
	req, _ := http.NewRequest("HEAD", url, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := client.Do(req)
	if err != nil {
		return filepath.Base(url) // Fallback auf URL-Endung
	}
	defer resp.Body.Close()

	// 1. Content-Disposition prüfen
	disposition := resp.Header.Get("Content-Disposition")
	if disposition != "" {
		if _, params, err := mime.ParseMediaType(disposition); err == nil {
			if filename, ok := params["filename"]; ok {
				return filename
			}
		}
	}

	// 2. Content-Type prüfen
	contentType := resp.Header.Get("Content-Type")
	ext := ""
	switch contentType {
	case "video/mp4":
		ext = ".mp4"
	case "image/jpeg":
		ext = ".jpg"
	case "application/pdf":
		ext = ".pdf"
		// ... weitere MIME-Typen
	}

	base := filepath.Base(url)
	if !strings.Contains(base, ".") && ext != "" {
		base += ext
	}

	return base
}

// ShowTUI - Zeigt die Terminal-Oberfläche und gibt Konfiguration zurück
func ShowTUI() (*DownloadConfig, bool) {
	app := tview.NewApplication()
	config := &DownloadConfig{
		Workers:   2,
		LimitKB:   0,
		OutputDir: "./downloads",
		URLs:      []string{},
		Names:     []string{}, // Custom Namen initialisieren
	}

	cancelled := false

	// Header
	header := tview.NewTextView()
	header.SetText(" 🚀 Go Download Manager - TUI Interface").
		SetTextAlign(tview.AlignCenter).
		SetTextColor(tcell.ColorYellow).
		SetBackgroundColor(tcell.ColorDarkBlue)

	// Input Fields (ohne SetFieldWidth für automatische Skalierung)
	urlInput := tview.NewInputField()
	urlInput.SetLabel("Download URL: ").
		SetBorder(true).
		SetTitle(" ➕ URL hinzufügen (Enter = hinzufügen) ")

	workersInput := tview.NewInputField()
	workersInput.SetLabel("Workers: ").
		SetText(strconv.Itoa(config.Workers)).
		SetBorder(true).
		SetTitle("# Worker ")

	outputDirInput := tview.NewInputField()
	outputDirInput.SetLabel("Output Dir: ").
		SetText(config.OutputDir).
		SetBorder(true).
		SetTitle(" 📁 Ausgabe-Verzeichnis ")

	limitInput := tview.NewInputField()
	limitInput.SetLabel("KB/s: ").
		SetText(strconv.Itoa(config.LimitKB)).
		SetBorder(true).
		SetTitle("Limit")

	// URL List
	urlList := tview.NewList()
	urlList.SetBorder(true).
		SetTitle(" 📋 Download URLs (d = löschen, F2 = Name vergeben) ").
		SetTitleAlign(tview.AlignLeft)

	// Status
	statusView := tview.NewTextView()
	statusView.SetText("").
		SetBorder(true).
		SetTitle(" 💬 Status ")

	// Layout Variables (müssen vor den Funktionen deklariert werden)
	var rootFlex *tview.Flex

	// Update URL List Display
	updateURLList := func() {
		urlList.Clear()
		for i, url := range config.URLs {
			displayText := fmt.Sprintf("%d. %s", i+1, url)
			if i < len(config.Names) && config.Names[i] != "" {
				displayText += fmt.Sprintf(" → [yellow]%s[white]", config.Names[i])
			}
			urlList.AddItem(displayText, "", 0, nil)
		}
	}

	// Name Input Modal Function
	showNameInput := func(urlIndex int) {
		nameInput := tview.NewInputField()
		nameInput.SetLabel("Orndername: ").
			SetBorder(true).
			SetTitle(" 📝 Dateinamen vergeben (Enter=Speichern) ").
			SetTitleAlign(tview.AlignCenter)

		// Aktueller Name falls vorhanden
		if urlIndex < len(config.Names) && config.Names[urlIndex] != "" {
			nameInput.SetText(config.Names[urlIndex])
		}

		nameInput.SetDoneFunc(func(key tcell.Key) {
			switch key {
			case tcell.KeyEnter:
				newName := strings.TrimSpace(nameInput.GetText())
				// Namen-Array erweitern falls nötig
				for len(config.Names) <= urlIndex {
					config.Names = append(config.Names, "")
				}
				config.Names[urlIndex] = newName
				updateURLList()
				app.SetRoot(rootFlex, true)
				app.SetFocus(urlList)
			}
		})

		// Modal Container
		nameModal := tview.NewFlex().
			SetDirection(tview.FlexRow).
			AddItem(tview.NewBox(), 0, 1, false).
			AddItem(tview.NewFlex().
				AddItem(tview.NewBox(), 0, 1, false).
				AddItem(nameInput, 50, 0, true).
				AddItem(tview.NewBox(), 0, 1, false), 3, 0, true).
			AddItem(tview.NewBox(), 0, 1, false)

		app.SetRoot(nameModal, true)
		app.SetFocus(nameInput)
	}
	updateStatus := func() {
		status := fmt.Sprintf("📊 %d URLs | Workers: %d", len(config.URLs), config.Workers)
		if config.LimitKB > 0 {
			status += fmt.Sprintf(" | Limit: %d KB/s", config.LimitKB)
		}
		status += fmt.Sprintf(" | Output: %s", config.OutputDir)
		statusView.SetText(status)
	}

	// Input Field Handlers
	urlInput.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			url := strings.TrimSpace(urlInput.GetText())
			if url != "" {
				config.URLs = append(config.URLs, url)
				detectedName := detectFilename(url)
				config.Names = append(config.Names, detectedName)
				urlInput.SetText("")
				updateURLList()
			}
		}
	})

	workersInput.SetChangedFunc(func(text string) {
		if val, err := strconv.Atoi(text); err == nil && val > 0 {
			config.Workers = val
			updateStatus()
		}
	})

	outputDirInput.SetChangedFunc(func(text string) {
		if strings.TrimSpace(text) != "" {
			config.OutputDir = strings.TrimSpace(text)
			updateStatus()
		}
	})

	limitInput.SetChangedFunc(func(text string) {
		if val, err := strconv.Atoi(text); err == nil && val >= 0 {
			config.LimitKB = val
			updateStatus()
		}
	})

	// URL List Handler
	urlList.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		index := urlList.GetCurrentItem()
		if event.Rune() == 'd' {
			if index >= 0 && index < len(config.URLs) {
				config.URLs = append(config.URLs[:index], config.URLs[index+1:]...)
				config.Names = append(config.Names[:index], config.Names[index+1:]...)
				updateURLList()
				updateStatus()
			}
		} else if event.Key() == tcell.KeyF2 {
			if index >= 0 && index < len(config.URLs) {
				// Name eingeben Modal
				showNameInput(index)
			}
		}
		return event
	})

	// Buttons
	startButton := tview.NewButton("🚀 Download starten (F12)")
	startButton.SetSelectedFunc(func() {
		if len(config.URLs) == 0 {
			return
		}
		app.Stop()
	})

	quitButton := tview.NewButton("❌ Beenden (ESC)")
	quitButton.SetSelectedFunc(func() {
		cancelled = true
		app.Stop()
	})

	buttonFlex := tview.NewFlex()
	buttonFlex.AddItem(startButton, 0, 1, false).
		AddItem(tview.NewBox(), 2, 0, false).
		AddItem(quitButton, 0, 1, false)

	// Settings Layout (responsive - passen sich der Fenstergröße an)
	settingsRow1 := tview.NewFlex().
		AddItem(urlInput, 0, 4, false).       // 4 Teile für URL (größer)
		AddItem(tview.NewBox(), 1, 0, false). // 1 Zeichen Abstand
		AddItem(workersInput, 0, 1, false)    // 1 Teil für Workers (kleiner)

	settingsRow2 := tview.NewFlex().
		AddItem(outputDirInput, 0, 4, false). // 4 Teile für Output Dir (größer)
		AddItem(tview.NewBox(), 1, 0, false). // 1 Zeichen Abstand
		AddItem(limitInput, 0, 1, false)      // 1 Teil für Limit (kleiner)

	// Main Layout
	mainContent := tview.NewFlex()
	mainContent.SetDirection(tview.FlexRow).
		AddItem(settingsRow1, 3, 0, true).
		AddItem(settingsRow2, 3, 0, false).
		AddItem(urlList, 0, 1, false).
		AddItem(buttonFlex, 3, 0, false).
		AddItem(statusView, 3, 0, false)

	rootFlex = tview.NewFlex()
	rootFlex.SetDirection(tview.FlexRow).
		AddItem(header, 1, 0, false).
		AddItem(mainContent, 0, 1, true)

	// Tab Order for Input Fields
	inputFields := []tview.Primitive{
		urlInput,
		workersInput,
		outputDirInput,
		limitInput,
		urlList,
		startButton,
		quitButton,
	}

	currentInputIndex := 0

	// Global Key Handlers
	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyF2:
			// F2 für Name vergeben (nur wenn URL List fokussiert)
			if app.GetFocus() == urlList {
				index := urlList.GetCurrentItem()
				if index >= 0 && index < len(config.URLs) {
					showNameInput(index)
				}
			}
			return nil
		case tcell.KeyF12: // Download starten auf F12 verschoben
			if len(config.URLs) > 0 {
				app.Stop()
			}
			return nil
		case tcell.KeyEscape:
			cancelled = true
			app.Stop()
			return nil
		case tcell.KeyTab:
			// Cycle through input fields
			currentInputIndex = (currentInputIndex + 1) % len(inputFields)
			app.SetFocus(inputFields[currentInputIndex])
			return nil
		case tcell.KeyBacktab: // Shift+Tab
			// Cycle backwards through input fields
			currentInputIndex = (currentInputIndex - 1 + len(inputFields)) % len(inputFields)
			app.SetFocus(inputFields[currentInputIndex])
			return nil
		}
		return event
	})

	app.SetRoot(rootFlex, true)
	app.SetFocus(urlInput)
	updateStatus()
	updateURLList() // Initial URL List aktualisieren

	if err := app.Run(); err != nil {
		panic(err)
	}

	return config, !cancelled
}
