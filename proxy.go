package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"bytes"
	"log"
	"strings"
)

const (
	ollamaURL = "http://localhost:11434"
	apiKey    = "connect"
)

type ChatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type OllamaRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream"`
}

type OllamaResponse struct {
	Message Message `json:"message"`
	Done    bool    `json:"done"`
}

type OpenAIResponse struct {
	Choices []Choice `json:"choices"`
}

type Choice struct {
	Message      *Message `json:"message,omitempty"`
	Delta        *Message `json:"delta,omitempty"`
	FinishReason *string  `json:"finish_reason"`
}

func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer "+apiKey {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func modelsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"object": "list",
		"data":   []map[string]any{{"id": "granite4.1:3b", "object": "model"}},
	})
}

func chatHandler(w http.ResponseWriter, r *http.Request) {
	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	if req.Model == "" {
		req.Model = "granite4.1:3b"
	}

	ollamaReq := OllamaRequest{
		Model:    req.Model,
		Messages: req.Messages,
		Stream:   req.Stream,
	}

	body, _ := json.Marshal(ollamaReq)

	if req.Stream {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		resp, err := http.Post(ollamaURL+"/api/chat", "application/json", bytes.NewReader(body))
		if err != nil {
			http.Error(w, "Ollama error", http.StatusInternalServerError)
			return
		}
		defer resp.Body.Close()

		flusher, _ := w.(http.Flusher)
		scanner := bufio.NewScanner(resp.Body)

		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				continue
			}
			var ollamaResp OllamaResponse
			if err := json.Unmarshal([]byte(line), &ollamaResp); err != nil {
				continue
			}
			var finishReason *string
			if ollamaResp.Done {
				s := "stop"
				finishReason = &s
			}
			chunk := OpenAIResponse{
				Choices: []Choice{{
					Delta:        &Message{Content: ollamaResp.Message.Content},
					FinishReason: finishReason,
				}},
			}
			data, _ := json.Marshal(chunk)
			fmt.Fprintf(w, "data: %s\n\n", data)
			if flusher != nil {
				flusher.Flush()
			}
			if ollamaResp.Done {
				fmt.Fprintf(w, "data: [DONE]\n\n")
				if flusher != nil {
					flusher.Flush()
				}
				return
			}
		}
		return
	}

	resp, err := http.Post(ollamaURL+"/api/chat", "application/json", bytes.NewReader(body))
	if err != nil {
		http.Error(w, "Ollama error", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	lines := strings.Split(strings.TrimSpace(string(respBody)), "\n")
	lastLine := lines[len(lines)-1]

	var ollamaResp OllamaResponse
	if err := json.Unmarshal([]byte(lastLine), &ollamaResp); err != nil {
		http.Error(w, "Parse error", http.StatusInternalServerError)
		return
	}

	stop := "stop"
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(OpenAIResponse{
		Choices: []Choice{{
			Message:      &Message{Role: "assistant", Content: ollamaResp.Message.Content},
			FinishReason: &stop,
		}},
	})
}

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	http.HandleFunc("/v1/models", authMiddleware(modelsHandler))
	http.HandleFunc("/v1/chat/completions", authMiddleware(chatHandler))

	log.Println("Proxy listening on :7860")
	log.Fatal(http.ListenAndServe(":7860", nil))
}
