// Package models keeps the user's list of OpenRouter models, the defaults new
// sessions start with, and what OpenRouter's catalogue says each model can
// do.
package models

import (
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"math"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/timbrinded/girdle/internal/checkpoint"
	"github.com/timbrinded/girdle/internal/clip"
)

// KeyCatalogURL lists the OpenRouter models an API key can use, after the
// account's provider preferences, privacy settings and guardrails.
// CatalogURL lists every model, and needs no key.
const (
	KeyCatalogURL = "https://openrouter.ai/api/v1/models/user"
	CatalogURL    = "https://openrouter.ai/api/v1/models"
)

// Info is what OpenRouter's catalogue says about a model.
type Info struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// About is the first sentence of OpenRouter's description.
	About string `json:"about,omitempty"`
	// Created is when OpenRouter added the model, in Unix seconds.
	Created int64 `json:"created,omitzero"`
	Context int   `json:"context,omitzero"`
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
	// ForKey is set when the list is the one for the user's API key,
	// rather than every model OpenRouter has.
	ForKey bool   `json:"for_key,omitzero"`
	Models []Info `json:"models"`
	// byID indexes Models for Lookup, which the picker calls for every row
	// it draws. Fetch and Store.Catalog build it.
	byID map[string]int
}

// indexed returns c with its models indexed by ID.
func (c Catalog) indexed() Catalog {
	c.byID = make(map[string]int, len(c.Models))
	for i, m := range c.Models {
		c.byID[m.ID] = i
	}
	return c
}

// Lookup finds a model in the catalogue.
func (c Catalog) Lookup(id string) (Info, bool) {
	i, ok := c.byID[id]
	if c.byID == nil {
		i = slices.IndexFunc(c.Models, func(m Info) bool { return m.ID == id })
		ok = i >= 0
	}
	if !ok {
		return Info{}, false
	}
	return c.Models[i], true
}

// Listed reports that the key can use model id, as far as the catalogue
// knows: an empty catalogue withdraws nothing.
func (c Catalog) Listed(id string) bool {
	_, ok := c.Lookup(id)
	return ok || len(c.Models) == 0
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
	Description   string `json:"description"`
	Created       int64  `json:"created"`
	ContextLength int    `json:"context_length"`
	Pricing       struct {
		Prompt     string `json:"prompt"`
		Completion string `json:"completion"`
	} `json:"pricing"`
	SupportedParameters []string `json:"supported_parameters"`
	Reasoning           *struct {
		// SupportedEfforts is a list of efforts, null when the model takes
		// every effort, or absent (see info).
		SupportedEfforts jsontext.Value `json:"supported_efforts"`
	} `json:"reasoning"`
	ExpirationDate string `json:"expiration_date"`
}

// FetchForKey downloads the models key can use. If OpenRouter won't give
// that list, it falls back to every model, and ForKey is false.
func FetchForKey(ctx context.Context, client *http.Client, key string) (Catalog, error) {
	c, err := Fetch(ctx, client, KeyCatalogURL, key)
	if err == nil {
		c.ForKey = true
		return c, nil
	}
	if c, err2 := Fetch(ctx, client, CatalogURL, ""); err2 == nil {
		return c, nil
	}
	return Catalog{}, err
}

// Fetch downloads OpenRouter's catalogue from url, with key if it's set.
func Fetch(ctx context.Context, client *http.Client, url, key string) (Catalog, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Catalog{}, err
	}
	req.Header.Set("User-Agent", "girdle")
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
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
	return c.indexed(), nil
}

func (r rawModel) info() Info {
	m := Info{
		ID:      r.ID,
		Name:    r.Name,
		About:   firstSentence(r.Description),
		Created: r.Created,
		Context: r.ContextLength,
		Tools:   slices.Contains(r.SupportedParameters, "tools"),
		Input:   perMillion(r.Pricing.Prompt),
		Output:  perMillion(r.Pricing.Completion),
		Expires: r.ExpirationDate,
	}
	// reasoning.supported_efforts lists the efforts a model takes, or is
	// null when it takes every effort. A model that takes the reasoning
	// parameter without listing efforts, such as Claude Opus 4.5, gets any
	// effort turned into a thinking budget by OpenRouter; a router such as
	// openrouter/auto passes it on to the model it picks. Either may be sent
	// any effort, as Girdle did before it read the catalogue.
	var efforts jsontext.Value
	if r.Reasoning != nil {
		efforts = r.Reasoning.SupportedEfforts
	}
	switch {
	case efforts.Kind() == '[':
		var supported []string
		_ = json.Unmarshal(efforts, &supported)
		for _, e := range checkpoint.Efforts {
			if slices.Contains(supported, string(e)) {
				m.Efforts = append(m.Efforts, e)
			}
		}
	case efforts.Kind() == 'n',
		slices.Contains(r.SupportedParameters, "reasoning"),
		slices.Contains(r.SupportedParameters, "reasoning_effort"):
		m.Efforts = checkpoint.Efforts
	}
	return m
}

// firstSentence keeps a description short enough for one line.
func firstSentence(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if before, _, ok := strings.Cut(s, ". "); ok {
		s = before + "."
	}
	return clip.Head(s, 200)
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
