#!/bin/bash
# build.sh - Kompiliert die TUI zu einer ausführbaren Datei

echo "🚀 Kompiliere Go Download Manager TUI..."

# Binary für aktuelles System kompilieren
go build -o dmanager main.go TUI.go

if [ $? -eq 0 ]; then
    echo "✅ Erfolgreich kompiliert!"
    echo "📁 Ausführbare Datei: ./dmanager"
    echo "🎯 Starten mit: ./dmanager"
    
    # Optional: Binary ausführbar machen (Linux/Mac)
    chmod +x dmanager

    cp dmanager /usr/local/bin/
else
    echo "❌ Kompilierung fehlgeschlagen!"
    exit 1
fi
