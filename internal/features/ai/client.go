package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// ChatClient memanggil OpenAI Chat Completions ({OPENAI_ENDPOINT}/chat/completions).
type ChatClient struct {
	endpoint string
	apiKey   string
	model    string
	http     *http.Client
}

// NewChatClient membuat ChatClient.
func NewChatClient(endpoint, apiKey, model string, hc *http.Client) *ChatClient {
	return &ChatClient{endpoint: endpoint, apiKey: apiKey, model: model, http: hc}
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
}

// Complete mengirim prompt system + user dan mengembalikan isi balasan pertama.
func (c *ChatClient) Complete(ctx context.Context, system, user string) (string, error) {
	// chatRequest hanya berisi string/angka, sehingga Marshal tidak dapat gagal.
	body, _ := json.Marshal(chatRequest{
		Model: c.model,
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
		Temperature: 0.7,
	})

	url := strings.TrimRight(c.endpoint, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	res, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("layanan AI mengembalikan status %d", res.StatusCode)
	}

	var payload struct {
		Choices []struct {
			Message chatMessage `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return "", err
	}
	if len(payload.Choices) == 0 {
		return "", errors.New("layanan AI tidak mengembalikan pilihan jawaban")
	}
	return payload.Choices[0].Message.Content, nil
}
