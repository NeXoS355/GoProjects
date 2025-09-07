#!/bin/bash
# build.sh - Kompiliert die TUI zu einer ausführbaren Datei

echo "🚀 Kompiliere Go PerformanceAnalyzer..."

# Binary für aktuelles System kompilieren
go build -o perfAnalyzer .

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
