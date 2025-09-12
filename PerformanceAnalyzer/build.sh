#!/bin/bash

echo "🚀 compiling Go Project..."

# for modern cpu models
GOOS=linux GOARCH=amd64 go build -o perfAnalyzer .
# for older cpu models
GOOS=linux GOARCH=amd64 GOAMD64=v1 go build -o perfAnalyzer_legacy .

if [ $? -eq 0 ]; then
  echo "✅ compiled successfull!"
  chmod +x perfAnalyzer
  echo "installing in /usr/local/bin/"
  cp perfAnalyzer /usr/local/bin/
else
  echo "❌ compilation failed!"
  exit 1
fi
