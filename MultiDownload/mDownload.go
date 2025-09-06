package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"golang.org/x/time/rate"
	"golang.org/x/term"
	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
)

// Job beschreibt einen einzelnen Download
type Job struct {
	URL      string
	Index    int
	Size     int64
	Filename string // Neues Feld für benutzerdefinierten Dateinamen
}

// RateLimitedReader implementiert Rate Limiting für io.Reader
type RateLimitedReader struct {
	reader  io.Reader
	limiter *rate.Limiter
}

func NewRateLimitedReader(reader io.Reader, limiter *rate.Limiter) *RateLimitedReader {
	return &RateLimitedReader{
		reader:  reader,
		limiter: limiter,
	}
}

func (r *RateLimitedReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if err != nil {
		return n, err // Fehler sofort zurückgeben
	}

	if n > 0 && r.limiter != nil {
		// Verwende einfachen WaitN ohne Context - blockiert bis Tokens verfügbar
		r.limiter.WaitN(context.Background(), n)
	}
	return n, nil
}

func getSafeFilename(url string, customName string, outputDir string) string {
	var base string

	// Verwende benutzerdefinierten Namen falls vorhanden
	if customName != "" {
		base = customName
	} else {
		base = filepath.Base(url)
		if base == "" || base == "/" {
			base = "download.bin"
		}
	}

	fullPath := filepath.Join(outputDir, base)

	// Wenn Datei existiert, Nummer anhängen
	if _, err := os.Stat(fullPath); err == nil {
		ext := filepath.Ext(base)
		name := strings.TrimSuffix(base, ext)
		for i := 1; ; i++ {
			newName := fmt.Sprintf("%s_%d%s", name, i, ext)
			newPath := filepath.Join(outputDir, newName)
			if _, err := os.Stat(newPath); os.IsNotExist(err) {
				return newPath
			}
		}
	}
	return fullPath
}

func worker(id int, jobs <-chan Job, wg *sync.WaitGroup, bars []*mpb.Bar, p *mpb.Progress, limiter *rate.Limiter, outputDir string) {
	defer wg.Done()
	for job := range jobs {
		err := downloadFile(job.URL, job.Filename, bars[job.Index], job.Size, limiter, outputDir)
		if err != nil {
			displayName := job.Filename
			if displayName == "" {
				displayName = filepath.Base(job.URL)
			}
			p.Write([]byte(fmt.Sprintf("❌ Fehler bei %s: %v\n", displayName, err)))
		}
	}
}

func downloadFile(url string, customName string, bar *mpb.Bar, size int64, limiter *rate.Limiter, outputDir string) error {
	client := &http.Client{}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	// Füge einen User-Agent hinzu, der von den meisten Servern akzeptiert wird
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) "+
	"AppleWebKit/537.36 (KHTML, like Gecko) "+
	"Chrome/122.0.0.0 Safari/537.36")

	// Optional: manchmal helfen diese Header zusätzlich
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP Fehler: %s", resp.Status)
	}

	filename := getSafeFilename(url, customName, outputDir)

	out, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer out.Close()

	var reader io.Reader = resp.Body
	if limiter != nil {
		reader = NewRateLimitedReader(resp.Body, limiter)
	}

	proxyReader := bar.ProxyReader(reader)
	_, err = io.Copy(out, proxyReader)
	return err
}


func getTerminalWidth() int {
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		return 80
	}
	return width
}

func main() {
	workers := flag.Int("w", 1, "Anzahl paralleler Downloads")
	outputDir := flag.String("o", ".", "Download-Ordner")
	limitKB := flag.Int("l", 0, "Bandwidth limit in KB/s (0 = kein Limit)")
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		fmt.Println("Benutzung: mDownload [Optionen] <URLs>")
		fmt.Println()
		fmt.Println("Optionen:")
		fmt.Println("  -w	Anzahl parallele Downloads (default: 1)")
		fmt.Println("  -l	Geschwindigkeit in KB/s (default: 0 = unbegrenzt)")
		fmt.Println("  -o	Ausgabeordner (default: .)")
		fmt.Println()
		fmt.Println("Date		Printf(url)inamen-Format:")
		fmt.Println("  URL@Dateiname   (z.B. https://example.com/file.zip@myname.zip)")
		fmt.Println("  URL             (automatischer Dateiname)")
		fmt.Println()
		fmt.Println("Beispiele:")
		fmt.Println("  mDownload https://example.com/file.zip@renamed.zip")
		fmt.Println("  mDownload -w 3 https://site1.com/a.zip@custom.zip https://site2.com/b.zip")
		fmt.Println("  mDownload -l 1000 https://example.com/big.zip@download.zip")
		return
	}

	// Parse URLs und Dateinamen - verwende @ als Trenner
	urls := make([]string, 0, len(args))
	names := make([]string, 0, len(args))

	for _, arg := range args {
		if strings.Contains(arg, "@") {
			parts := strings.SplitN(arg, "@", 2)
			if len(parts) == 2 && parts[1] != "" {
				urls = append(urls, parts[0])
				names = append(names, parts[1])
			} else {
				urls = append(urls, parts[0])
				names = append(names, "")
			}
		} else {
			urls = append(urls, arg)
			names = append(names, "")
		}
	}

	// Erstelle Output-Ordner falls er nicht existiert
	if err := os.MkdirAll(*outputDir, 0755); err != nil {
		fmt.Printf("Kann Output-Ordner nicht erstellen: %v\n", err)
		return
	}

	// Erstelle Rate Limiter (geteilt zwischen allen Workers)
	var limiter *rate.Limiter
	if *limitKB > 0 {
		// Konvertiere KB/s zu Bytes/s
		bytesPerSecond := rate.Limit(*limitKB * 1024)
		limiter = rate.NewLimiter(bytesPerSecond, *limitKB*1024) // Burst = 1 Sekunde
		fmt.Printf("Bandwidth limitiert auf %d KB/s\n", *limitKB)
	}

	termWidth := getTerminalWidth()

	// 1. Schritt: Alle Dateigrößen abfragen
	sizes := make([]int64, len(urls))
	displayNames := make([]string, len(urls))
	maxNameLen := 0

	fmt.Println("Ermittle Dateigrößen...")
	for i, url := range urls {
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Head(url)
		if err != nil {
			displayName := names[i]
			if displayName == "" {
				displayName = filepath.Base(url)
			}
			fmt.Printf("⚠ Kann Größe von %s nicht abfragen: %v\n", displayName, err)
			sizes[i] = 1
		} else {
			sizes[i] = resp.ContentLength
			if sizes[i] <= 0 {
				sizes[i] = 1
			}
		}

		// Display-Name für Progress Bar bestimmen
		if names[i] != "" {
			displayNames[i] = names[i]
		} else {
			displayNames[i] = filepath.Base(url)
		}

		if len(displayNames[i]) > maxNameLen {
			maxNameLen = len(displayNames[i])
		}
	}

	if maxNameLen > 25 {
		maxNameLen = 25
	}

	// 2. Schritt: mpb Progress Container erstellen (ORIGINAL Konfiguration)
	p := mpb.New(
		mpb.WithRefreshRate(180*time.Millisecond),
	)

	// 3. Schritt: Bars für jeden Download erstellen (ORIGINAL Format)
	bars := make([]*mpb.Bar, len(urls))
	for i, name := range displayNames {
		displayName := name
		if len(displayName) > maxNameLen {
			displayName = displayName[:maxNameLen-3] + "..."
		}

		nameWidth := maxNameLen + 3
		counterWidth := 20
		percentWidth := 5
		etaWidth := 8
		barChars := 3

		availableBarWidth := termWidth - nameWidth - counterWidth - percentWidth - etaWidth - barChars
		if availableBarWidth < 10 {
			availableBarWidth = 10
		}

		bars[i] = p.New(
			sizes[i],
		  mpb.BarStyle().Lbound("[").Filler("█").Tip("").Padding("░").Rbound("]"),
				mpb.BarWidth(availableBarWidth),
				mpb.PrependDecorators(
					decor.Name(fmt.Sprintf("%-*s | ", maxNameLen, displayName), decor.WCSyncSpace),
						      decor.CountersKibiByte("% .1f / % .1f"),
				),
		  mpb.AppendDecorators(
			  decor.Percentage(decor.WC{W: 4}),
				       decor.OnComplete(decor.EwmaETA(decor.ET_STYLE_MMSS, 90, decor.WC{W: 6}), " ✓"),
		  ),
		)
	}

	// 4. Schritt: Worker starten (ORIGINAL Logik)
	jobs := make(chan Job, len(urls))
	var wg sync.WaitGroup

	for w := 1; w <= *workers; w++ {
		wg.Add(1)
		go worker(w, jobs, &wg, bars, p, limiter, *outputDir)
	}

	// 5. Schritt: Jobs einfügen (ORIGINAL Weise)
	for i, url := range urls {
		fmt.Printf("URL:" + url)
		jobs <- Job{
			URL:      url,
			Index:    i,
			Size:     sizes[i],
			Filename: names[i],
		}
	}
	close(jobs)

	wg.Wait()
	p.Wait()

	fmt.Printf("Alle Downloads abgeschlossen in: %s\n", *outputDir)
}
