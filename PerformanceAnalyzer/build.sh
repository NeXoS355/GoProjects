#!/bin/bash
# build.sh - Kompiliert die TUI zu einer ausführbaren Datei

echo "🚀 Kompiliere Go PerformanceAnalyzer..."

# Binary für aktuelles System kompilieren
# for modern cpu models
GOOS=linux GOARCH=amd64 go build -o perfAnalyzer .
# for older cpu models
GOOS=linux GOARCH=amd64 GOAMD64=v1 go build -o perfAnalyzer_legacy .

if [ $? -eq 0 ]; then
  echo "✅ Erfolgreich kompiliert!"
  echo "📁 Ausführbare Datei: ./perfAnalyzer"

  # Optional: Binary ausführbar machen (Linux/Mac)
  chmod +x perfAnalyzer

  cp perfAnalyzer /usr/local/bin/
else
  echo "❌ Kompilierung fehlgeschlagen!"
  exit 1
fi
