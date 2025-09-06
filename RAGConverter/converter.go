package main

import (
	"bytes"
	"crypto/md5"
	"encoding/json"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gocolly/colly"
	"github.com/google/uuid"
	"github.com/PuerkitoBio/goquery"
)

type DataChunk struct {
	ID      string      `json:"id"`
	Page    string      `json:"page"`
	Type    string      `json:"type"` // section | table | image
	Title   string      `json:"title,omitempty"`
	Content string      `json:"content,omitempty"`
	Table   *TableBlock `json:"table,omitempty"`
	Image   *ImageBlock `json:"image,omitempty"`
}

type TableBlock struct {
	Headers []string   `json:"headers"`
	Rows    [][]string `json:"rows"`
}

type ImageBlock struct {
	URL         string `json:"url"`
	Caption     string `json:"caption"`
	AltText     string `json:"alt_text"`
	Description string `json:"description"`
}

type OllamaRequest struct {
	Model  string   `json:"model"`
	Prompt string   `json:"prompt"`
	Images []string `json:"images"`
	Stream bool     `json:"stream"`
}

type OllamaResponse struct {
	Response string `json:"response"`
	Done     bool   `json:"done"`
}

// ---- Bild herunterladen und lokal speichern ----
func downloadImage(imageURL, filepath string) error {
	// Timeout für Downloads
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	resp, err := client.Get(imageURL)
	if err != nil {
		return fmt.Errorf("fehler beim Download von %s: %v", imageURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP-Fehler %d beim Download von %s", resp.StatusCode, imageURL)
	}

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("fehler beim Lesen der Bilddaten: %v", err)
	}

	err = ioutil.WriteFile(filepath, data, 0644)
	if err != nil {
		return fmt.Errorf("fehler beim Speichern der Datei %s: %v", filepath, err)
	}

	return nil
}

// ---- Eindeutigen Dateinamen für Bilder generieren ----
func generateImageFilename(imageURL string) string {
	// MD5-Hash der URL für eindeutige Dateinamen
	hash := md5.Sum([]byte(imageURL))
	hashStr := fmt.Sprintf("%x", hash)

	// Dateierweiterung aus URL extrahieren
	u, err := url.Parse(imageURL)
	if err != nil {
		return hashStr + ".jpg" // Fallback
	}

	ext := filepath.Ext(u.Path)
	if ext == "" {
		ext = ".jpg" // Fallback für Bilder ohne Erweiterung
	}

	return hashStr + ext
}

// ---- LLaVA über Ollama aufrufen (mit lokalem Dateipfad) ----
func analyzeImageWithLLaVA(imagePath string,desc string) (string, error) {
	apiURL := "http://localhost:11434/api/generate"

	// Prüfen ob die Datei existiert
	if _, err := os.Stat(imagePath); os.IsNotExist(err) {
		return "", fmt.Errorf("bilddatei nicht gefunden: %s", imagePath)
	}

	data, err := ioutil.ReadFile(imagePath)
	if err != nil {
		return "", fmt.Errorf("fehler beim Lesen der Bilddatei: %v", err)
	}

	b64 := base64.StdEncoding.EncodeToString(data)

	reqData := OllamaRequest{
		Model:  "llava:7b",
		Prompt: "Beschreibe dieses Bild detailliert auf Deutsch. Erkläre was zu sehen ist, welche Objekte, Personen oder Konzepte dargestellt werden. Versuche anhand des alt Namens den Kontext herzustellen: " + desc,
		Images: []string{b64}, // Nur base64, kein data:-Präfix nötig
		Stream: false,
	}

	jsonData, err := json.Marshal(reqData)
	if err != nil {
		return "", fmt.Errorf("fehler beim JSON-Marshaling: %v", err)
	}

	client := &http.Client{
		Timeout: 120 * time.Second, // Längere Timeout für LLaVA
	}

	resp, err := client.Post(apiURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("fehler beim API-Aufruf: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API-Fehler: Status %d", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("fehler beim Lesen der Antwort: %v", err)
	}

	// Ollama kann streamen oder nicht - beide Fälle abdecken
	var output strings.Builder
	lines := bytes.Split(body, []byte("\n"))

	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		var res OllamaResponse
		if err := json.Unmarshal(line, &res); err == nil {
			output.WriteString(res.Response)
			if res.Done {
				break
			}
		}
	}

	result := strings.TrimSpace(output.String())
	if result == "" {
		return "", fmt.Errorf("keine Antwort von LLaVA erhalten")
	}

	return result, nil
}

// ---- Seitenslug erzeugen ----
func makeSlug(pageURL string) string {
	preURL := strings.Split(pageURL, ":")
	postURL := preURL[0] + ":" + preURL[1]
	fmt.Printf(postURL)
	u, err := url.Parse(postURL)
	if err != nil {
		return "page"
	}
	parts := strings.Split(u.Path, "/")
	slug := parts[len(parts)-1]
	if slug == "" && len(parts) > 1 {
		slug = parts[len(parts)-2]
	}
	return strings.ReplaceAll(slug, " ", "_")
}

// ---- Prüfen ob Ollama läuft ----
func checkOllamaConnection() error {
	resp, err := http.Get("http://localhost:11434/api/tags")
	if err != nil {
		return fmt.Errorf("ollama ist nicht erreichbar: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ollama antwortet mit Status %d", resp.StatusCode)
	}

	return nil
}

func main() {
	// URL als Kommandozeilenargument oder Standard
	//startURL := "https://de.wikipedia.org/wiki/Go_(Programmiersprache)"
	startURL := "https://nexos-srv.de:4443"
	if len(os.Args) > 1 {
		startURL = os.Args[1]
	}

	fmt.Printf("🌐 Scrape Seite: %s\n", startURL)
	var results []DataChunk
	var currentTitle string

	// Ollama-Verbindung prüfen
	fmt.Println("🔍 Prüfe Ollama-Verbindung...")
	if err := checkOllamaConnection(); err != nil {
		log.Printf("⚠️ Warnung: %v", err)
		log.Println("Bilder werden ohne KI-Beschreibung verarbeitet.")
	} else {
		fmt.Println("✅ Ollama ist erreichbar")
	}

	// Verzeichnis für temporäre Bilder erstellen
	tempDir := "./temp_images"
	os.MkdirAll(tempDir, 0755)
	//defer os.RemoveAll(tempDir) // Aufräumen am Ende

	c := colly.NewCollector(
		colly.AllowedDomains("nexos-srv.de:4443"),
				colly.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36"),
	)

	// Rate limiting
	c.Limit(&colly.LimitRule{
		DomainGlob:  "*",
	 Parallelism: 2,
	 Delay:       1 * time.Second,
	})

	c.OnHTML("body", func(e *colly.HTMLElement) {
		doc := e.DOM

		// ---------- TEXTE ----------
		doc.Find("h2, h3, p").Each(func(i int, s *goquery.Selection) {
			tag := goquery.NodeName(s)
			text := strings.TrimSpace(s.Text())
			if text == "" {
				return
			}

			if tag == "h2" || tag == "h3" {
				currentTitle = text
				return
			}

			// Absatz mit Titel als Kontext
			chunk := DataChunk{
				ID:      uuid.New().String(),
					   Page:    startURL,
					   Type:    "section",
					   Title:   currentTitle,
					   Content: text,
			}
			results = append(results, chunk)
		})

		// ---------- TABELLEN ----------
		doc.Find("table").Each(func(i int, table *goquery.Selection) {
			var headers []string
			var rows [][]string

			table.Find("tr th").Each(func(i int, th *goquery.Selection) {
				headers = append(headers, strings.TrimSpace(th.Text()))
			})

			table.Find("tr").Each(func(i int, tr *goquery.Selection) {
				var row []string
				tr.Find("td").Each(func(i int, td *goquery.Selection) {
					row = append(row, strings.TrimSpace(td.Text()))
				})
				if len(row) > 0 {
					rows = append(rows, row)
				}
			})

			if len(headers) > 0 || len(rows) > 0 {
				chunk := DataChunk{
					ID:   uuid.New().String(),
				       Page: startURL,
				       Type: "table",
				       Table: &TableBlock{
					       Headers: headers,
					       Rows:    rows,
				       },
				}
				results = append(results, chunk)
			}
		})

		// ---------- BILDER ----------
		fmt.Println("🖼️ Verarbeite Bilder...")
		doc.Find("img").Each(func(i int, img *goquery.Selection) {
			src, exists := img.Attr("src")
			if !exists || src == "" {
				return
			}

			alt, _ := img.Attr("alt")

			// Caption aus verschiedenen Quellen suchen
			caption := ""
			if figcaption := img.ParentsFiltered("figure").Find("figcaption"); figcaption.Length() > 0 {
				caption = strings.TrimSpace(figcaption.Text())
			}

			// Relative URLs zu absoluten URLs konvertieren
			if strings.HasPrefix(src, "//") {
				src = "https:" + src
			} else if strings.HasPrefix(src, "/") {
				u, _ := url.Parse(startURL)
				src = fmt.Sprintf("%s://%s%s", u.Scheme, u.Host, src)
			} else {
				src = startURL + "/" + src
			}

			// Sehr kleine Bilder (Icons, etc.) überspringen
			width, _ := img.Attr("width")
			height, _ := img.Attr("height")
			if (width != "" && width < "50") || (height != "" && height < "50") {
				return
			}

			fmt.Printf("📥 Lade Bild herunter: %s\n", src)

			// Bild herunterladen
			filename := generateImageFilename(src)
			localPath := filepath.Join(tempDir, filename)

			err := downloadImage(src, localPath)
			if err != nil {
				fmt.Printf("⚠️ Fehler beim Download: %v\n", err)
				// Trotzdem Chunk erstellen, nur ohne KI-Beschreibung
				chunk := DataChunk{
					ID:   uuid.New().String(),
				     Page: startURL,
				     Type: "image",
				     Image: &ImageBlock{
					     URL:         src,
					     AltText:     alt,
					     Caption:     caption,
					     Description: "Fehler beim Bilddownload: " + err.Error(),
				     },
				}
				results = append(results, chunk)
				return
			}

			// Bildanalyse via LLaVA
			fmt.Printf("🤖 Analysiere Bild mit LLaVA...\n")
			fmt.Printf("🤖 ALT Description: " + alt + "\n")
			desc, err := analyzeImageWithLLaVA(localPath, alt)
			if err != nil {
				fmt.Printf("⚠️ Fehler bei LLaVA-Analyse: %v\n", err)
				desc = fmt.Sprintf("LLaVA-Analyse fehlgeschlagen: %v", err)
			} else {
				fmt.Printf("✅ Bildanalyse erfolgreich\n")
			}

			chunk := DataChunk{
				ID:   uuid.New().String(),
				     Page: startURL,
				     Type: "image",
				     Image: &ImageBlock{
					     URL:         src,
					     AltText:     alt,
					     Caption:     caption,
					     Description: desc,
				     },
			}
			results = append(results, chunk)
		})
	})

	fmt.Printf("🌐 Scrape Seite: %s\n", startURL)
	err := c.Visit(startURL)
	if err != nil {
		log.Fatal(err)
	}

	// ---- Ausgabe ----
	//slug := makeSlug(startURL)
	outfile := filepath.Join(".","/output.json")

	file, err := os.Create(outfile)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	err = encoder.Encode(results)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("✅ %d Datenelemente exportiert nach %s\n", len(results), outfile)
}
