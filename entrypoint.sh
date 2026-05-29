#!/bin/bash
set -e

ollama serve &
OLLAMA_PID=$!

echo "Waiting for Ollama..."
sleep 8

echo "Pulling granite4.1:3b..."
ollama pull granite4.1:3b

echo "Starting proxy..."
proxy &

echo "Ready."
wait $OLLAMA_PID
