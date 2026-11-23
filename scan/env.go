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

// syncXurlAuth hands X_BEARER_TOKEN to xurl, which only reads credentials
// from its own store (~/.xurl). App-only bearer auth is used because search
// needs no user context, and xurl v1.3.4 mis-signs OAuth1 requests whose
// query contains a space (401 on any multi-word search).
// The token goes over stdin so it never appears in the process list.
func syncXurlAuth(ctx context.Context) error {
	token := os.Getenv("X_BEARER_TOKEN")
	if token == "" {
		return fmt.Errorf("X_BEARER_TOKEN is not set (.env or shell)")
	}
	cmd := exec.CommandContext(ctx, "xurl", "auth", "app-only", "-")
	cmd.Stdin = strings.NewReader(token)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("xurl auth app-only: %w: %s", err, out)
	}
	return nil
}
