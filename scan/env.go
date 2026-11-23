package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// loadDotEnv copies KEY=VALUE lines into the process env. Variables already
// set in the shell win, so a one-off `ANTHROPIC_API_KEY=... go run` overrides.
func loadDotEnv(path string) error {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(strings.TrimPrefix(sc.Text(), "export "))
		key, val, ok := strings.Cut(line, "=")
		if !ok || strings.HasPrefix(line, "#") {
			continue
		}
		key, val = strings.TrimSpace(key), strings.Trim(strings.TrimSpace(val), `"'`)
		if _, set := os.LookupEnv(key); !set {
			os.Setenv(key, val)
		}
	}
	return sc.Err()
}

// syncXurlAuth hands credentials from the env to xurl, which only reads its
// own store (~/.xurl).
//   - search: app-only bearer token. Search needs no user context, and xurl
//     v1.3.4 mis-signs OAuth1 requests whose query has a space (401).
//     The token goes over stdin so it never appears in the process list.
//   - home timeline: OAuth1 user context (bearer cannot read it). xurl takes
//     these only as flags, so they are briefly visible in the process list.
func syncXurlAuth(ctx context.Context, search, timeline bool) error {
	if search {
		token := os.Getenv("X_BEARER_TOKEN")
		if token == "" {
			return fmt.Errorf("X_BEARER_TOKEN is not set (.env or shell)")
		}
		cmd := exec.CommandContext(ctx, "xurl", "auth", "app-only", "-")
		cmd.Stdin = strings.NewReader(token)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("xurl auth app-only: %w: %s", err, out)
		}
	}
	if timeline {
		return syncOAuth1(ctx)
	}
	return nil
}

var oauth1Keys = []string{"X_CONSUMER_KEY", "X_CONSUMER_SECRET", "X_ACCESS_TOKEN", "X_ACCESS_TOKEN_SECRET"}

func syncOAuth1(ctx context.Context) error {
	v := make([]string, len(oauth1Keys))
	for i, k := range oauth1Keys {
		if v[i] = os.Getenv(k); v[i] == "" {
			return fmt.Errorf("%s is not set; reading your following needs all four OAuth1 keys", k)
		}
	}
	out, err := exec.CommandContext(ctx, "xurl", "auth", "oauth1",
		"--consumer-key", v[0], "--consumer-secret", v[1],
		"--access-token", v[2], "--token-secret", v[3]).CombinedOutput()
	if err != nil {
		return fmt.Errorf("xurl auth oauth1: %w: %s", err, out)
	}
	return nil
}
