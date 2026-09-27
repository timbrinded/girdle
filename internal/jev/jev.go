// Package jev is a small client for TypeSafe's System One API.
//
// It sends a state and a map of typed questions, and returns calibrated
// answers. See https://docs.typesafe.ai/api.
package jev

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"
)

// DefaultModel is pinned so thresholds stay valid until we re-tune them.
const DefaultModel = "jev-1.13.0"

// DefaultBaseURL is TypeSafe's hosted API.
const DefaultBaseURL = "https://api.typesafe.ai"

// Question is one typed question. Build it with Noul, Choice or Score.
type Question struct {
	Type         string `json:"type"`
	Instructions any    `json:"instructions"`
	Criteria     any    `json:"criteria,omitempty"`
}

// Noul asks for the probability that a statement is true.
func Noul(instructions string) Question {
	return Question{Type: "noul", Instructions: instructions}
}

// Choice asks the model to pick one option. Keys are option names; values
// describe what each option means.
func Choice(instructions string, options map[string]string) Question {
	return Question{Type: "choice", Instructions: instructions, Criteria: options}
}

// Score asks the model to place the state on ordered levels, lowest first.
func Score(instructions string, levels ...string) Question {
	return Question{Type: "score", Instructions: instructions, Criteria: levels}
}

// Answer holds whichever fields apply to the question's type.
type Answer struct {
	Type          string             `json:"type"`
	Noul          float64            `json:"noul"`
	Choice        string             `json:"choice,omitempty"`
	Score         float64            `json:"score"`
	Probabilities map[string]float64 `json:"probabilities,omitempty"`
	Confidence    float64            `json:"confidence"`
}

// Usage is the token count Jev billed for a request.
type Usage struct {
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
}

// Response is Jev's answer to one request.
type Response struct {
	Model   string            `json:"model"`
	Answers map[string]Answer `json:"answers"`
	Usage   Usage             `json:"usage"`
}

// Client calls the System One endpoint.
type Client struct {
	BaseURL    string
	Model      string
	APIKey     string
	HTTP       *http.Client
	MaxRetries int
}

// ErrNoAPIKey is returned when TYPESAFE_API_KEY is not set.
var ErrNoAPIKey = errors.New("jev: TYPESAFE_API_KEY is not set")

// New returns a client for a System One endpoint at baseURL, such as
// OpenCode Zen's, which serves Jev under other model names.
func New(baseURL, model, key string) *Client {
	return &Client{BaseURL: baseURL, Model: model, APIKey: key, HTTP: &http.Client{Timeout: 20 * time.Second}, MaxRetries: 3}
}

// NewFromEnv builds a client from TYPESAFE_API_KEY and optional
// GIRDLE_JEV_MODEL and GIRDLE_JEV_URL overrides.
func NewFromEnv() (*Client, error) {
	key := os.Getenv("TYPESAFE_API_KEY")
	if key == "" {
		return nil, ErrNoAPIKey
	}
	return &Client{
		BaseURL:    cmp.Or(os.Getenv("GIRDLE_JEV_URL"), DefaultBaseURL),
		Model:      cmp.Or(os.Getenv("GIRDLE_JEV_MODEL"), DefaultModel),
		APIKey:     key,
		HTTP:       &http.Client{Timeout: 20 * time.Second},
		MaxRetries: 3,
	}, nil
}

// StatusError is a non-retryable HTTP error from the API.
type StatusError struct {
	Code int
	Body string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("jev: HTTP %d: %s", e.Code, e.Body)
}

type request struct {
	State     any                 `json:"state"`
	Model     string              `json:"model"`
	Questions map[string]Question `json:"questions"`
}

// Ask evaluates every question against the same state in one request.
// It retries rate limits, overloads and server errors with backoff.
func (c *Client) Ask(ctx context.Context, state any, questions map[string]Question) (*Response, error) {
	body, err := json.Marshal(request{State: state, Model: c.Model, Questions: questions})
	if err != nil {
		return nil, fmt.Errorf("jev: encode request: %w", err)
	}
	backoff := 500 * time.Millisecond
	var lastErr error
	for attempt := range c.MaxRetries + 1 {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
			backoff = min(backoff*2, 8*time.Second)
		}
		resp, retryAfter, err := c.do(ctx, body)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		if se, ok := errors.AsType[*StatusError](err); ok && !retryable(se.Code) {
			return nil, err
		}
		if retryAfter > 0 {
			backoff = retryAfter
		}
	}
	return nil, lastErr
}

func (c *Client) do(ctx context.Context, body []byte) (*Response, time.Duration, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/v1/systemone", bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("jev: %w", err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return nil, 0, fmt.Errorf("jev: read response: %w", err)
	}
	if res.StatusCode != http.StatusOK {
		var wait time.Duration
		if s, err := strconv.Atoi(res.Header.Get("Retry-After")); err == nil {
			wait = time.Duration(s) * time.Second
		}
		return nil, wait, &StatusError{Code: res.StatusCode, Body: truncate(string(raw), 300)}
	}
	var out Response
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, 0, fmt.Errorf("jev: decode response: %w", err)
	}
	return &out, 0, nil
}

func retryable(code int) bool {
	return code == http.StatusTooManyRequests || code == 529 || code >= 500
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
