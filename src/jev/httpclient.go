// Package jev evaluates TypeSafe questions with the Jev API.
package jev

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/t3snake/gocode/src/core"
)

type QuestionType string

const (
	Noul   QuestionType = "noul"
	Choice QuestionType = "choice"
	Score  QuestionType = "score"
)

// Question describes one primitive. Instructions can be a string, object, or array.
// Noul criteria are optional. Choice criteria are a map of options to descriptions.
// Score criteria are an ordered array of 2 to 10 level descriptions.
type Question struct {
	Type         QuestionType `json:"type"`
	Instructions any          `json:"instructions"`
	Criteria     any          `json:"criteria,omitempty"`
}

type JevRequestBody struct {
	State     any                 `json:"state"`
	Model     string              `json:"model"`
	Questions map[string]Question `json:"questions"`
}

// Answer contains the result for the question at the same index.
// Use Type to select Noul, Choice, or Score. Noul is a probability from 0 to 1.
// Choice and Score also include Probabilities and Confidence; Score includes Legend.
type Answer struct {
	Type          QuestionType       `json:"type"`
	Noul          float64            `json:"noul"`
	Choice        string             `json:"choice"`
	Score         float64            `json:"score"`
	Probabilities map[string]float64 `json:"probabilities"`
	Confidence    float64            `json:"confidence"`
	Legend        map[string]string  `json:"legend"`
}

type UsageJevResponse struct {
	InputTokens  uint32 `json:"input_tokens"`
	OutputTokens uint32 `json:"output_tokens"`
}

type JevResponse struct {
	Model   string            `json:"model"`
	Answers map[string]Answer `json:"answers"`
	Usage   UsageJevResponse  `json:"usage"`
}

// APIError contains the HTTP status and response body from a failed request.
type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("jev: HTTP %d: %s", e.StatusCode, e.Body)
}

// Client can be reused for concurrent requests. Set its fields before use.
type Client struct {
	http_client *http.Client
	model       string
	api_key     string
}

// NewClient reads TYPESAFE_API_KEY. Requests have a one-minute timeout by default.
func NewClient() (*Client, error) {
	key := strings.TrimSpace(os.Getenv("TYPESAFE_API_KEY"))
	if key == "" {
		return nil, fmt.Errorf("jev: TYPESAFE_API_KEY is not set")
	}
	return &Client{
		http_client: &http.Client{Timeout: 5 * time.Second},
		model:       core.JevModelName,
		api_key:     key,
	}, nil
}

// Evaluate sends one request and returns answers in question order.
// State can be a string, object, or array that encoding/json can encode.
// An empty question slice returns an empty answer slice without an HTTP request.
// Failed requests are not retried. Callers can use APIError to handle rate limits.
func (c *Client) Evaluate(ctx context.Context, state any, questions map[string]Question) (JevResponse, error) {
	if len(questions) == 0 {
		return JevResponse{}, nil
	}

	if c == nil || c.api_key == "" || c.http_client == nil {
		return JevResponse{}, fmt.Errorf("jev: initialize the client with NewClient")
	}

	if ctx == nil {
		return JevResponse{}, fmt.Errorf("jev: context is nil")
	}

	jev_body := JevRequestBody{
		State:     state,
		Model:     c.model,
		Questions: questions,
	}

	requestBody, err := json.Marshal(jev_body)

	if err != nil {
		return JevResponse{}, fmt.Errorf("jev: encode request: %w", err)
	}

	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		core.JevEndpoint,
		bytes.NewReader(requestBody),
	)

	if err != nil {
		return JevResponse{}, fmt.Errorf("jev: create request: %w", err)
	}

	request.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.api_key))
	request.Header.Set("Content-Type", "application/json")

	response, err := c.http_client.Do(request)

	if err != nil {
		return JevResponse{}, fmt.Errorf("jev: send request: %w", err)
	}

	defer response.Body.Close()

	// Bound reads so an unexpected response cannot use unlimited memory.
	const maxResponseBytes = 16 << 20
	data, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return JevResponse{}, fmt.Errorf("jev: read response: %w", err)
	}

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return JevResponse{}, &APIError{StatusCode: response.StatusCode, Body: string(data)}
	}

	var result JevResponse

	if err := json.Unmarshal(data, &result); err != nil {
		return JevResponse{}, fmt.Errorf("jev: decode response: %w", err)
	}

	if len(result.Answers) != len(questions) {
		return JevResponse{}, fmt.Errorf("jev: got %d answers for %d questions", len(result.Answers), len(questions))
	}

	return result, nil
}
