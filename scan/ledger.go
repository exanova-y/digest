package main

import (
	"encoding/json"
	"errors"
	"os"
	"sort"
	"strings"
	"time"
)

// Stock is one row of stocks.json.
type Stock struct {
	Name                string  `json:"name"`
	Ticker              string  `json:"ticker"`
	TweetURL            string  `json:"tweet_url"`
	InvestingWindow     string  `json:"investing_window"` // horizon for mean and variance
	ExpectedVariance    float64 `json:"expected_variance"`
	ExpectedMean        float64 `json:"expected_mean"`
	Confidence          string  `json:"confidence"` // low | medium | high
	Comments            string  `json:"comments"`   // comma-separated cues
	TimestampLastEdited string  `json:"timestamp_last_edited"`
}

func loadLedger(path string) ([]Stock, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var rows []Stock
	return rows, json.Unmarshal(b, &rows)
}

// mergeLedger overwrites rows for tickers seen in this scan and keeps the rest,
// so a stock that goes quiet keeps its last estimate and its older timestamp.
func mergeLedger(old, fresh []Stock, now time.Time) []Stock {
	byTicker := map[string]Stock{}
	for _, s := range old {
		byTicker[s.Ticker] = s
	}
	for _, s := range fresh {
		s.Ticker = strings.ToUpper(strings.TrimPrefix(strings.TrimSpace(s.Ticker), "$"))
		if s.Ticker == "" {
			continue // never let a blank model row overwrite the ledger
		}
		s.TimestampLastEdited = now.Format(time.RFC3339)
		byTicker[s.Ticker] = s
	}
	rows := make([]Stock, 0, len(byTicker))
	for _, s := range byTicker {
		rows = append(rows, s)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Ticker < rows[j].Ticker })
	return rows
}

// saveLedger writes via a temp file so a crash never leaves half a JSON file.
func saveLedger(path string, rows []Stock) error {
	b, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
