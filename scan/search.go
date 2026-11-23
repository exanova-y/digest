package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os/exec"
	"time"
)

// Post is the subset of an X post that the extractor needs.
type Post struct {
	ID            string `json:"id"`
	Text          string `json:"text"`
	CreatedAt     string `json:"created_at"`
	PublicMetrics struct {
		Likes   int `json:"like_count"`
		Reposts int `json:"retweet_count"`
		Quotes  int `json:"quote_count"`
	} `json:"public_metrics"`
}

// searchPosts queries all of X (not the user's home timeline, which rarely
// carries market talk) for the most relevant matches inside the window.
// The raw endpoint is used because `xurl search` exposes neither start_time
// nor sort_order; xurl still handles OAuth.
func searchPosts(ctx context.Context, query string, window time.Duration, n int) ([]Post, error) {
	q := url.Values{
		"query":        {query},
		"start_time":   {time.Now().UTC().Add(-window).Format(time.RFC3339)},
		"sort_order":   {"relevancy"},
		"max_results":  {fmt.Sprint(n)},
		"tweet.fields": {"created_at,public_metrics"},
	}
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "xurl", "--auth", "app", "/2/tweets/search/recent?"+q.Encode())
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	// xurl prints X API error bodies (e.g. 402 credits depleted) to stdout.
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("xurl: %w: %s%s", err, stderr.String(), stdout.String())
	}
	return decodePosts(stdout.Bytes())
}

func decodePosts(body []byte) ([]Post, error) {
	var resp struct {
		Data   []Post            `json:"data"`
		Errors []json.RawMessage `json:"errors"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode xurl output: %w", err)
	}
	if len(resp.Data) == 0 && len(resp.Errors) > 0 {
		return nil, fmt.Errorf("X API: %s", resp.Errors[0])
	}
	return dedupe(resp.Data), nil
}

// dedupe drops posts whose text was already seen; spam bots repost verbatim
// and would otherwise read to Claude as broad consensus.
func dedupe(posts []Post) []Post {
	seen := map[string]bool{}
	out := posts[:0]
	for _, p := range posts {
		if !seen[p.Text] {
			seen[p.Text] = true
			out = append(out, p)
		}
	}
	return out
}
