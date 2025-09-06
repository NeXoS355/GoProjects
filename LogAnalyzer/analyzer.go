package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"gopkg.in/yaml.v3"
)

// Config strukturen (unverändert)
type Config struct {
	Logs []LogConfig `yaml:"logs"`
}

type LogConfig struct {
	Path     string `yaml:"path"`
	Type     string `yaml:"type"`
	LogLevel string `yaml:"loglevel"`
	Color    string `yaml:"color"`
}

type LogEntry struct {
	Timestamp time.Time
	Source    string
	Severity  string
	Message   string
	Metadata  map[string]string
}

var levelOrder = map[string]int{
	"debug": 0,
	"info":  1,
	"warn":  2,
	"error": 3,
	"fatal": 4,
}

var colors = map[string]string{
	"red":     "#ff5555",
	"green":   "#50fa7b",
	"yellow":  "#f1fa8c",
	"blue":    "#8be9fd",
	"magenta": "#ff79c6",
	"cyan":    "#8be9fd",
	"white":   "#f8f8f2",
}

var severityColors = map[string]string{
	"debug": "#6272a4", // grau
	"info":  "#50fa7b", // grün
	"warn":  "#f1fa8c", // gelb
	"error": "#ff5555", // rot
	"fatal": "#ff79c6", // magenta
}

// Styles für die UI
var (
	titleStyle = lipgloss.NewStyle().
	Bold(true).
	Foreground(lipgloss.Color("#8be9fd")).
	MarginLeft(2)

	selectedStyle = lipgloss.NewStyle().
	Bold(true).
	Foreground(lipgloss.Color("#f8f8f2")).
	Background(lipgloss.Color("#44475a"))

	helpStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#6272a4"))

	logLineStyle = lipgloss.NewStyle().
	MarginLeft(1)
)

// List Item für Log-Dateien
type logFileItem struct {
	config LogConfig
}

func (i logFileItem) FilterValue() string { return i.config.Path }
func (i logFileItem) Title() string       { return i.config.Path }
func (i logFileItem) Description() string {
	return fmt.Sprintf("Type: %s | Level: %s | Color: %s", i.config.Type, i.config.LogLevel, i.config.Color)
}

// Model für die Anwendung
type model struct {
	config     *Config
	list       list.Model
	viewport   viewport.Model
	logs       []string
	showLogs   bool
	parsers    map[string]Parser
	currentLog LogConfig
	keys       keyMap
}

type keyMap struct {
	Up     key.Binding
	Down   key.Binding
	Enter  key.Binding
	Back   key.Binding
	Quit   key.Binding
	Help   key.Binding
	Reload key.Binding
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Help, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Enter},
		{k.Back, k.Reload, k.Quit},
	}
}

var keys = keyMap{
	Up: key.NewBinding(
		key.WithKeys("up", "k"),
			   key.WithHelp("↑/k", "move up"),
	),
	Down: key.NewBinding(
		key.WithKeys("down", "j"),
			     key.WithHelp("↓/j", "move down"),
	),
	Enter: key.NewBinding(
		key.WithKeys("enter"),
			      key.WithHelp("enter", "select"),
	),
	Back: key.NewBinding(
		key.WithKeys("esc", "b"),
			     key.WithHelp("esc", "back"),
	),
	Quit: key.NewBinding(
		key.WithKeys("q", "ctrl+c"),
			     key.WithHelp("q", "quit"),
	),
	Help: key.NewBinding(
		key.WithKeys("?"),
			     key.WithHelp("?", "toggle help"),
	),
	Reload: key.NewBinding(
		key.WithKeys("r"),
			       key.WithHelp("r", "reload"),
	),
}

func initialModel(cfg *Config) model {
	// Liste der Log-Dateien erstellen
	items := make([]list.Item, len(cfg.Logs))
	for i, logCfg := range cfg.Logs {
		items[i] = logFileItem{config: logCfg}
	}

	l := list.New(items, list.NewDefaultDelegate(), 80, 20)
	l.Title = "Log Analyzer - Wähle eine Log-Datei"
	l.SetShowStatusBar(false)

	vp := viewport.New(80, 20)
	vp.Style = lipgloss.NewStyle().
	BorderStyle(lipgloss.RoundedBorder()).
	BorderForeground(lipgloss.Color("#44475a")).
	PaddingLeft(2).
	PaddingRight(2)

	// Parser Registry
	parsers := map[string]Parser{
		"apache":    &ApacheParser{},
		"nextcloud": &NextcloudParser{},
	}

	return model{
		config:   cfg,
		list:     l,
		viewport: vp,
		showLogs: false,
		parsers:  parsers,
		keys:     keys,
	}
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
		case tea.WindowSizeMsg:
			if !m.showLogs {
				m.list.SetWidth(msg.Width)
				m.list.SetHeight(msg.Height - 4)
			} else {
				m.viewport.Width = msg.Width - 4
				m.viewport.Height = msg.Height - 4
			}
			return m, nil

		case tea.KeyMsg:
			if m.showLogs {
				switch {
					case key.Matches(msg, m.keys.Back):
						m.showLogs = false
						m.logs = nil
						return m, nil
					case key.Matches(msg, m.keys.Reload):
						m.loadLogFile(m.currentLog)
						return m, nil
					case key.Matches(msg, m.keys.Quit):
						return m, tea.Quit
				}
				var cmd tea.Cmd
				m.viewport, cmd = m.viewport.Update(msg)
				return m, cmd
			} else {
				switch {
					case key.Matches(msg, m.keys.Enter):
						if item, ok := m.list.SelectedItem().(logFileItem); ok {
							m.currentLog = item.config
							m.loadLogFile(item.config)
							m.showLogs = true
						}
						return m, nil
					case key.Matches(msg, m.keys.Quit):
						return m, tea.Quit
				}
				var cmd tea.Cmd
				m.list, cmd = m.list.Update(msg)
				return m, cmd
			}
	}

	if !m.showLogs {
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m *model) loadLogFile(cfg LogConfig) {
	parser, ok := m.parsers[cfg.Type]
	if !ok {
		m.logs = []string{fmt.Sprintf("Fehler: Kein Parser für Typ '%s' gefunden", cfg.Type)}
		m.viewport.SetContent(strings.Join(m.logs, "\n"))
		return
	}

	file, err := os.Open(cfg.Path)
	if err != nil {
		m.logs = []string{fmt.Sprintf("Fehler beim Öffnen der Datei: %v", err)}
		m.viewport.SetContent(strings.Join(m.logs, "\n"))
		return
	}
	defer file.Close()

	var logLines []string
	logLines = append(logLines, titleStyle.Render(fmt.Sprintf("==> %s (%s, Level: %s)", cfg.Path, cfg.Type, cfg.LogLevel)))
	logLines = append(logLines, "")

	scanner := bufio.NewScanner(file)
	count := 0
	for scanner.Scan() {
		line := scanner.Text()
		entry, err := parser.Parse(line)
		if err != nil {
			continue
		}

		if !shouldLog(cfg.LogLevel, entry.Severity) {
			continue
		}

		// Styling der Log-Zeile
		ts := entry.Timestamp.Format("02.01.2006 15:04")

		sourceStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colors[cfg.Color])).
		Bold(true)

		severityStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(severityColors[entry.Severity])).
		Bold(true)

		logLine := fmt.Sprintf("[%s] %s | %s | %s",
				       sourceStyle.Render(entry.Source),
				       ts,
			 severityStyle.Render(entry.Severity),
				       entry.Message,
		)

		logLines = append(logLines, logLineStyle.Render(logLine))
		count++

		// Begrenzen auf 1000 Zeilen für Performance
		if count >= 1000 {
			logLines = append(logLines, "")
			logLines = append(logLines, helpStyle.Render("... (weitere Einträge wurden abgeschnitten, maximal 1000 Zeilen angezeigt)"))
			break
		}
	}

	if err := scanner.Err(); err != nil {
		logLines = append(logLines, fmt.Sprintf("Fehler beim Lesen: %v", err))
	}

	if count == 0 {
		logLines = append(logLines, helpStyle.Render("Keine Log-Einträge gefunden oder alle wurden gefiltert."))
	}

	m.logs = logLines
	m.viewport.SetContent(strings.Join(logLines, "\n"))
}

func (m model) View() string {
	if !m.showLogs {
		help := helpStyle.Render("Pfeiltasten: Navigation | Enter: Auswählen | q: Beenden | ?: Hilfe")
		return lipgloss.JoinVertical(
			lipgloss.Left,
			m.list.View(),
					     help,
		)
	}

	help := helpStyle.Render("Pfeiltasten: Scrollen | r: Neu laden | Esc: Zurück | q: Beenden")
	return lipgloss.JoinVertical(
		lipgloss.Left,
		m.viewport.View(),
				     help,
	)
}

// Parser Interface und Implementierungen (unverändert)
type Parser interface {
	Parse(line string) (LogEntry, error)
}

type ApacheParser struct{}

func (p *ApacheParser) Parse(line string) (LogEntry, error) {
	return LogEntry{
		Timestamp: time.Now(),
		Source:    "apache",
		Severity:  "info",
		Message:   line,
		Metadata:  map[string]string{},
	}, nil
}

type NextcloudLog struct {
	ReqID      string                 `json:"reqId"`
	Level      int                    `json:"level"`
	Time       string                 `json:"time"`
	RemoteAddr string                 `json:"remoteAddr"`
	User       string                 `json:"user"`
	App        string                 `json:"app"`
	Method     string                 `json:"method"`
	URL        string                 `json:"url"`
	Message    string                 `json:"message"`
	UserAgent  string                 `json:"userAgent"`
	Version    string                 `json:"version"`
	Data       map[string]interface{} `json:"data"`
}

type NextcloudParser struct{}

func (p *NextcloudParser) Parse(line string) (LogEntry, error) {
	var nc NextcloudLog
	if err := json.Unmarshal([]byte(line), &nc); err != nil {
		return LogEntry{}, err
	}

	t, err := time.Parse(time.RFC3339, nc.Time)
	if err != nil {
		t = time.Now()
	}

	severity := map[int]string{
		0: "debug",
		1: "info",
		2: "warn",
		3: "error",
		4: "fatal",
	}
	sev := severity[nc.Level]

	return LogEntry{
		Timestamp: t,
		Source:    "nextcloud",
		Severity:  sev,
		Message:   nc.Message,
		Metadata: map[string]string{
			"remoteAddr": nc.RemoteAddr,
			"user":       nc.User,
			"app":        nc.App,
		},
	}, nil
}

// Helper functions (unverändert)
func shouldLog(minLevel, entryLevel string) bool {
	return levelOrder[entryLevel] >= levelOrder[minLevel]
}

func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func main() {
	cfg, err := loadConfig("config.yaml")
	if err != nil {
		log.Fatal(err)
	}

	p := tea.NewProgram(
		initialModel(cfg),
			    tea.WithAltScreen(),
			    tea.WithMouseCellMotion(),
	)

	if _, err := p.Run(); err != nil {
		fmt.Printf("Fehler beim Starten der Anwendung: %v", err)
		os.Exit(1)
	}
}
