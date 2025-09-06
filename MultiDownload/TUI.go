// Enhanced UI with Video URL Extraction
// ui.go
package main

import (
	"context"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
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

// VideoExtractResult enthält das Ergebnis der Video-Extraktion
type VideoExtractResult struct {
	VideoURLs []string
	PageTitle string
	Error     error
}

// extractVideoURLs extrahiert Video-URLs von einer Webseite
func extractVideoURLs(pageURL string) VideoExtractResult {
	result := VideoExtractResult{
		VideoURLs: []string{},
		PageTitle: "",
		Error:     nil,
	}

	parsedURL, err := url.Parse(pageURL)
	if err != nil {
		result.Error = fmt.Errorf("ungültige URL: %v", err)
		return result
	}

	mainDomain := getMainDomain(parsedURL.Host)

	// Timeout für die Extraktion
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Headless Chromium-Kontext
	ctx, cancelCtx := chromedp.NewContext(ctx)
	defer cancelCtx()

	var mediaURLs []string
	var pageTitle string

	// Listener für Netzwerk-Responses
	chromedp.ListenTarget(ctx, func(ev interface{}) {
		switch ev := ev.(type) {
		case *network.EventResponseReceived:
			if ev.Response != nil && strings.HasPrefix(ev.Response.MimeType, "video") {
				u := ev.Response.URL
				if strings.HasSuffix(getMainDomain(getDomain(u)), mainDomain) && !contains(mediaURLs, u) {
					mediaURLs = append(mediaURLs, u)
				}
			}
		}
	})

	// Netzwerk überwachen
	if err := chromedp.Run(ctx, network.Enable()); err != nil {
		result.Error = fmt.Errorf("fehler beim Aktivieren von Network: %v", err)
		return result
	}

	// Seite laden
	err = chromedp.Run(ctx,
		chromedp.Navigate(pageURL),
		chromedp.Sleep(3*time.Second),
		chromedp.Title(&pageTitle),
		chromedp.Sleep(5*time.Second),
	)
	if err != nil {
		result.Error = fmt.Errorf("fehler beim Laden der Seite: %v", err)
		return result
	}

	result.VideoURLs = mediaURLs
	result.PageTitle = sanitizeFilename(pageTitle)
	return result
}

// sanitizeFilename bereinigt einen String für die Verwendung als Dateiname
func sanitizeFilename(filename string) string {
	// Gefährliche Zeichen entfernen/ersetzen
	reg := regexp.MustCompile(`[<>:"/\\|?*]`)
	clean := reg.ReplaceAllString(filename, "_")

	// Mehrfache Unterstriche reduzieren
	reg2 := regexp.MustCompile(`_+`)
	clean = reg2.ReplaceAllString(clean, "_")

	// Leerzeichen trimmen und begrenzen
	clean = strings.TrimSpace(clean)
	if len(clean) > 100 {
		clean = clean[:100]
	}

	return clean
}

// contains prüft, ob ein Slice ein Element enthält
func contains(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

// getDomain extrahiert die Domain einer URL
func getDomain(u string) string {
	parsed, err := url.Parse(u)
	if err != nil {
		return ""
	}
	return parsed.Host
}

// getMainDomain extrahiert die Hauptdomain, z.B. "example.org" aus "videos.example.org"
func getMainDomain(host string) string {
	parts := strings.Split(host, ".")
	if len(parts) < 2 {
		return host
	}
	return strings.Join(parts[len(parts)-2:], ".")
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

// isVideoURL prüft, ob eine URL wahrscheinlich ein Video ist
func isVideoURL(u string) bool {
	// Einfache Heuristik basierend auf URL-Endungen
	videoExtensions := []string{".mp4", ".avi", ".mkv", ".mov", ".wmv", ".flv", ".webm", ".m3u8"}
	lowerURL := strings.ToLower(u)

	for _, ext := range videoExtensions {
		if strings.Contains(lowerURL, ext) {
			return true
		}
	}

	// Bekannte Video-Plattformen
	videoSites := []string{"youtube.com", "vimeo.com", "twitch.tv", "dailymotion.com"}
	for _, site := range videoSites {
		if strings.Contains(lowerURL, site) {
			return true
		}
	}

	return false
}

// addURL fügt entweder direkt die URL hinzu oder extrahiert Videos von einer Seite (asynchron)
func addURL(u string, extractVideo bool, config *DownloadConfig, updateList func(), app *tview.Application, statusView *tview.TextView) {
	if extractVideo {
		go func(pageURL string) {
			// Ladeanzeige
			app.QueueUpdateDraw(func() {
				statusView.SetText(fmt.Sprintf("⏳ Extrahiere Videos von %s ...", pageURL))
			})

			// Extraktion im Hintergrund
			res := extractVideoURLs(pageURL)

			// Ergebnis zurück in die UI
			app.QueueUpdateDraw(func() {
				if res.Error != nil {
					statusView.SetText(fmt.Sprintf("❌ Fehler bei %s: %v", pageURL, res.Error))
					return
				}
				for _, v := range res.VideoURLs {
					config.URLs = append(config.URLs, v)
					config.Names = append(config.Names, res.PageTitle)
				}
				updateList()
				statusView.SetText(fmt.Sprintf("✅ %d Videos von %s extrahiert", len(res.VideoURLs), pageURL))
			})
		}(u)
	} else {
		// Direkte URL hinzufügen
		config.URLs = append(config.URLs, u)
		config.Names = append(config.Names, detectFilename(u))
		updateList()
		app.QueueUpdateDraw(func() {
			statusView.SetText(fmt.Sprintf("➕ URL hinzugefügt: %s", u))
		})
	}
}

// ShowTUI - Zeigt die Terminal-Oberfläche und gibt Konfiguration zurück
func ShowTUI() (*DownloadConfig, bool) {
	app := tview.NewApplication()
	config := &DownloadConfig{
		Workers:   2,
		LimitKB:   0,
		OutputDir: "./downloads",
		URLs:      []string{},
		Names:     []string{},
	}

	cancelled := false

	// Header
	header := tview.NewTextView()
	header.SetText(" 🚀 Go Download Manager - Smart Video Extractor").
		SetTextAlign(tview.AlignCenter).
		SetTextColor(tcell.ColorYellow).
		SetBackgroundColor(tcell.ColorDarkBlue)

	// Input Fields
	urlInput := tview.NewInputField()
	urlInput.SetLabel("URL/Video-Seite: ").
		SetBorder(true).
		SetTitle(" ➕ URL hinzufügen (Enter = Auto-Extrakt, Ctrl+Enter = direkt) ")

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

	// Layout Variables
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

	updateStatus := func() {
		status := fmt.Sprintf("📊 %d URLs | Workers: %d", len(config.URLs), config.Workers)
		if config.LimitKB > 0 {
			status += fmt.Sprintf(" | Limit: %d KB/s", config.LimitKB)
		}
		status += fmt.Sprintf(" | Output: %s", config.OutputDir)
		statusView.SetText(status)
	}

	// Name Input Modal Function
	showNameInput := func(urlIndex int) {
		nameInput := tview.NewInputField()
		nameInput.SetLabel("Dateiname: ").
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

	// Input Field Handlers
	urlInput.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEnter {
			url := strings.TrimSpace(urlInput.GetText())
			if url != "" {
				extractVideo := (event.Modifiers() & tcell.ModCtrl) == 0
				addURL(url, extractVideo, config, updateURLList, app, statusView)
				urlInput.SetText("")
			}
			return nil
		}
		return event
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

	// Settings Layout
	settingsRow1 := tview.NewFlex().
		AddItem(urlInput, 0, 4, false).
		AddItem(tview.NewBox(), 1, 0, false).
		AddItem(workersInput, 0, 1, false)

	settingsRow2 := tview.NewFlex().
		AddItem(outputDirInput, 0, 4, false).
		AddItem(tview.NewBox(), 1, 0, false).
		AddItem(limitInput, 0, 1, false)

	// Main Layout
	mainContent := tview.NewFlex()
	mainContent.SetDirection(tview.FlexRow).
		AddItem(settingsRow1, 3, 0, true).
		AddItem(settingsRow2, 3, 0, false).
		AddItem(urlList, 0, 1, false).
		AddItem(buttonFlex, 3, 0, false).
		AddItem(statusView, 4, 0, false) // Etwas höher für mehrzeilige Meldungen

	rootFlex = tview.NewFlex()
	rootFlex.SetDirection(tview.FlexRow).
		AddItem(header, 1, 0, false).
		AddItem(mainContent, 0, 1, true)

	// Tab Order
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
			if app.GetFocus() == urlList {
				index := urlList.GetCurrentItem()
				if index >= 0 && index < len(config.URLs) {
					showNameInput(index)
				}
			}
			return nil
		case tcell.KeyF12:
			if len(config.URLs) > 0 {
				app.Stop()
			}
			return nil
		case tcell.KeyEscape:
			cancelled = true
			app.Stop()
			return nil
		case tcell.KeyTab:
			currentInputIndex = (currentInputIndex + 1) % len(inputFields)
			app.SetFocus(inputFields[currentInputIndex])
			return nil
		case tcell.KeyBacktab:
			currentInputIndex = (currentInputIndex - 1 + len(inputFields)) % len(inputFields)
			app.SetFocus(inputFields[currentInputIndex])
			return nil
		}
		return event
	})

	app.SetRoot(rootFlex, true)
	app.SetFocus(urlInput)
	updateStatus()
	updateURLList()

	if err := app.Run(); err != nil {
		panic(err)
	}

	return config, !cancelled
}
