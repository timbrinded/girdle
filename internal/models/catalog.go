// Package models keeps the user's list of OpenRouter models, the defaults new
// sessions start with, and what OpenRouter's catalogue says each model can
// do.
package models

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"math"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/timbrinded/girdle/internal/checkpoint"
)

// CatalogURL lists OpenRouter's models. It needs no API key.
const CatalogURL = "https://openrouter.ai/api/v1/models"

// Info is what OpenRouter's catalogue says about a model.
type Info struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Context int    `json:"context,omitzero"`
	// Tools reports that the model accepts tool calls, which Girdle needs.
	Tools bool `json:"tools,omitzero"`
	// Efforts are the reasoning efforts the model accepts, lowest first.
	// Empty means it has no effort setting.
	Efforts []checkpoint.Effort `json:"efforts,omitempty"`
	// Input and Output are dollars per million tokens. They are negative
	// for a router, whose price depends on the model it picks.
	Input  float64 `json:"input,omitzero"`
	Output float64 `json:"output,omitzero"`
	// Expires is the date OpenRouter has announced it will withdraw the
	// model, if it has.
	Expires string `json:"expires,omitempty"`
}

// Catalog is OpenRouter's list of models, as fetched at one time.
type Catalog struct {
	Fetched time.Time `json:"fetched,omitzero"`
	Models  []Info    `json:"models"`
}

// Lookup finds a model in the catalogue.
func (c Catalog) Lookup(id string) (Info, bool) {
	i := slices.IndexFunc(c.Models, func(m Info) bool { return m.ID == id })
	if i < 0 {
		return Info{}, false
	}
	return c.Models[i], true
}

// Gone reports that the catalogue is known and doesn't list id: OpenRouter
// has withdrawn the model, or never had it.
func (c Catalog) Gone(id string) bool {
	_, ok := c.Lookup(id)
	return len(c.Models) > 0 && !ok
}

// Efforts are the reasoning efforts model id accepts, lowest first. A model
// the catalogue doesn't know is assumed to accept every effort, so Girdle
// sends what it is asked to, as it did before it read the catalogue.
func (c Catalog) Efforts(id string) []checkpoint.Effort {
	if m, ok := c.Lookup(id); ok {
		return m.Efforts
	}
	return checkpoint.Efforts
}

// rawModel is one model in OpenRouter's listing.
type rawModel struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	ContextLength int    `json:"context_length"`
	Pricing       struct {
		Prompt     string `json:"prompt"`
		Completion string `json:"completion"`
	} `json:"pricing"`
	SupportedParameters []string `json:"supported_parameters"`
	Reasoning           *struct {
		SupportedEfforts []string `json:"supported_efforts"`
	} `json:"reasoning"`
	ExpirationDate string `json:"expiration_date"`
}

// Fetch downloads OpenRouter's catalogue.
func Fetch(ctx context.Context, client *http.Client, url string) (Catalog, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Catalog{}, err
	}
	req.Header.Set("User-Agent", "girdle")
	resp, err := client.Do(req)
	if err != nil {
		return Catalog{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Catalog{}, fmt.Errorf("OpenRouter's model list: %s", resp.Status)
	}
	var listing struct {
		Data []rawModel `json:"data"`
	}
	if err := json.UnmarshalRead(resp.Body, &listing); err != nil {
		return Catalog{}, fmt.Errorf("OpenRouter's model list: %w", err)
	}
	c := Catalog{Fetched: time.Now(), Models: make([]Info, 0, len(listing.Data))}
	for _, r := range listing.Data {
		c.Models = append(c.Models, r.info())
	}
	return c, nil
}

func (r rawModel) info() Info {
	m := Info{
		ID:      r.ID,
		Name:    r.Name,
		Context: r.ContextLength,
		Tools:   slices.Contains(r.SupportedParameters, "tools"),
		Input:   perMillion(r.Pricing.Prompt),
		Output:  perMillion(r.Pricing.Completion),
		Expires: r.ExpirationDate,
	}
	switch {
	case r.Reasoning != nil:
		for _, e := range checkpoint.Efforts {
			if slices.Contains(r.Reasoning.SupportedEfforts, string(e)) {
				m.Efforts = append(m.Efforts, e)
			}
		}
	case slices.Contains(r.SupportedParameters, "reasoning_effort"):
		// A router such as openrouter/auto takes an effort and passes it on
		// to whichever model it picks, so any effort may apply.
		m.Efforts = checkpoint.Efforts
	}
	return m
}

// perMillion turns OpenRouter's price per token into dollars per million
// tokens. A router's price is -1.
func perMillion(perToken string) float64 {
	v, err := strconv.ParseFloat(perToken, 64)
	if err != nil || v < 0 {
		return -1
	}
	// Rounded to a billionth of a dollar, so $0.0000001 a token reads as
	// $0.1 a million, not 0.09999999999999999.
	return math.Round(v*1e15) / 1e9
}
