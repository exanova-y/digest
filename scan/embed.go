package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"os"
	"strings"
)

// OpenAI's text-embedding-3-small at 512 dims, the model and size of
// garden/scripts/embed-curius.py, so scores here and on the garden share one
// scale. Reached through OpenRouter, which proxies OpenAI's embeddings API.
// A run embeds ~10 pages.
const (
	embedURL   = "https://openrouter.ai/api/v1/embeddings"
	embedModel = "openai/text-embedding-3-small"
	embedDims  = 512
	embedCache = ".embed-cache.json"
)

// embedder caches vectors by URL: anchors and source pages rarely change, and
// the feed already skips links it has. Delete .embed-cache.json to refresh.
type embedder struct {
	key   string
	cache struct {
		Model   string               `json:"model"`
		Vectors map[string][]float64 `json:"vectors"`
	}
}

func newEmbedder() (*embedder, error) {
	e := &embedder{key: os.Getenv("OPENROUTER_API_KEY")}
	if e.key == "" {
		return nil, errors.New("OPENROUTER_API_KEY is not set (.env or shell); the reads worldview check needs it")
	}
	if b, err := os.ReadFile(embedCache); err == nil {
		json.Unmarshal(b, &e.cache)
	}
	if e.cache.Model != embedModel || e.cache.Vectors == nil {
		e.cache.Model, e.cache.Vectors = embedModel, map[string][]float64{}
	}
	return e, nil
}

// vectors returns a unit vector per page, embedding only uncached URLs.
func (e *embedder) vectors(ctx context.Context, pages []page) ([][]float64, error) {
	var todo []page
	for _, p := range pages {
		if _, ok := e.cache.Vectors[p.URL]; !ok {
			todo = append(todo, p)
		}
	}
	if len(todo) > 0 {
		inputs := make([]string, len(todo))
		for i, p := range todo {
			inputs[i] = p.Title + "\n\n" + p.Text
		}
		got, err := e.call(ctx, inputs)
		if err != nil {
			return nil, err
		}
		for i, p := range todo {
			e.cache.Vectors[p.URL] = unit(got[i])
		}
		if b, err := json.Marshal(e.cache); err == nil {
			os.WriteFile(embedCache, b, 0o644)
		}
	}
	out := make([][]float64, len(pages))
	for i, p := range pages {
		out[i] = e.cache.Vectors[p.URL]
	}
	return out, nil
}

func (e *embedder) call(ctx context.Context, inputs []string) ([][]float64, error) {
	body, _ := json.Marshal(map[string]any{"input": inputs, "model": embedModel, "dimensions": embedDims})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, embedURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.key)
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out struct {
		Data []struct {
			Embedding []float64 `json:"embedding"`
			Index     int       `json:"index"`
		} `json:"data"`
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("embeddings: HTTP %d: %w", resp.StatusCode, err)
	}
	if resp.StatusCode != http.StatusOK || len(out.Data) != len(inputs) {
		return nil, fmt.Errorf("embeddings: HTTP %d: %s", resp.StatusCode, out.Error.Message)
	}
	vecs := make([][]float64, len(inputs))
	for _, d := range out.Data {
		vecs[d.Index] = d.Embedding
	}
	return vecs, nil
}

// unit normalises so cosine similarity is a plain dot product. OpenAI
// returns unit vectors, but the cache must stay correct if the model changes.
func unit(v []float64) []float64 {
	var n float64
	for _, x := range v {
		n += x * x
	}
	n = math.Sqrt(n)
	if n == 0 {
		return v
	}
	out := make([]float64, len(v))
	for i, x := range v {
		out[i] = x / n
	}
	return out
}

// nearest returns the highest cosine similarity to any anchor and its index:
// a page only has to sound like one of the writers, not their average.
func nearest(v []float64, anchors [][]float64) (float64, int) {
	best, at := -1.0, -1
	for i, a := range anchors {
		var dot float64
		for j := range v {
			dot += v[j] * a[j]
		}
		if dot > best {
			best, at = dot, i
		}
	}
	return best, at
}

func readAnchors(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var urls []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if line := strings.TrimSpace(sc.Text()); line != "" && !strings.HasPrefix(line, "#") {
			urls = append(urls, line)
		}
	}
	return urls, sc.Err()
}
