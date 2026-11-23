package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go/v2"
)

const systemPrompt = `You read a batch of recent posts from an X search for market talk and pick out stocks that are notable right now: heavy discussion, news, earnings, unusual moves, or strong conviction from posters.
For each stock:
- source_post_id: the id of the single post that best shows why it is notable (copy it exactly from the <post id=...> tag).
- investing_window: the horizon over which the catalyst in the posts is likely to play out (a momentum spike is short; a fundamental shift is longer).
- expected_mean: expected return over that investing_window, in percent (e.g. 2.5 means +2.5%).
- expected_variance: variance of that return over the same window, in percent squared (e.g. 64 means an 8% standard deviation).
Base estimates on the posts plus your general knowledge of the company's volatility. Skip crypto, indexes and ETFs, and skip tickers mentioned only in passing.
- confidence: how well the posts support these estimates. high = many independent posters and a concrete, verifiable catalyst; medium = clear catalyst but thin or one-sided discussion; low = hype, rumor, or a single source.
- comments: the cues behind the pick as comma-separated values, under 30 words in total, e.g. "CEO reinstated, +31% intraday, sector-wide semis rally, bears call relief rally".
Return an empty list if nothing qualifies.`

// pick is the model's row. It names a post id rather than a URL so the link
// is built in code from posts we actually fetched, never invented.
type pick struct {
	Name             string  `json:"name"`
	Ticker           string  `json:"ticker"`
	SourcePostID     string  `json:"source_post_id"`
	InvestingWindow  string  `json:"investing_window"`
	ExpectedMean     float64 `json:"expected_mean"`
	ExpectedVariance float64 `json:"expected_variance"`
	Confidence       string  `json:"confidence"`
	Comments         string  `json:"comments"`
}

// stockSchema is raw JSON, not a Go map: maps marshal keys alphabetically,
// which forced "comments" first and "name"/"ticker" last. In that order the
// model returned an all-blank row in 3 of 4 runs; property order matters.
var stockSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "stocks": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "name": {"type": "string"},
          "ticker": {"type": "string"},
          "source_post_id": {"type": "string"},
          "investing_window": {"type": "string", "enum": ["1 day", "3 days", "1 week", "2 weeks", "1 month", "3 months", "6 months", "1 year"]},
          "expected_mean": {"type": "number"},
          "expected_variance": {"type": "number"},
          "confidence": {"type": "string", "enum": ["low", "medium", "high"]},
          "comments": {"type": "string"}
        },
        "required": ["name", "ticker", "source_post_id", "investing_window", "expected_mean", "expected_variance", "confidence", "comments"],
        "additionalProperties": false
      }
    }
  },
  "required": ["stocks"],
  "additionalProperties": false
}`)

// extractStocks makes one structured-output call; the schema guarantees the
// reply decodes straight into []Stock.
func extractStocks(ctx context.Context, posts []Post) ([]Stock, error) {
	if len(posts) == 0 {
		return nil, nil
	}
	client := anthropic.NewClient()
	resp, err := client.Beta.Messages.New(ctx, anthropic.BetaMessageNewParams{
		Model:     "claude-opus-5-5",
		MaxTokens: 16000,
		Betas:     []anthropic.AnthropicBeta{anthropic.AnthropicBetaServerSideFallback2026_07_01},
		Fallbacks: anthropic.BetaFallbacksParamOfDefault(),
		System:    []anthropic.BetaTextBlockParam{{Text: systemPrompt}},
		OutputConfig: anthropic.BetaOutputConfigParam{
			Effort: anthropic.BetaOutputConfigEffortHigh,
			Format: anthropic.BetaJSONOutputFormatParam{Schema: stockSchema},
		},
		Messages: []anthropic.BetaMessageParam{
			anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock(formatPosts(posts))),
		},
	})
	if err != nil {
		return nil, err
	}
	picks, err := decodePicks(resp)
	if err != nil {
		return nil, err
	}
	return toStocks(picks, posts), nil
}

// toStocks links each pick to its source post. A pick whose id is not among
// the fetched posts keeps an empty URL rather than a guessed one.
func toStocks(picks []pick, posts []Post) []Stock {
	fetched := map[string]bool{}
	for _, p := range posts {
		fetched[p.ID] = true
	}
	stocks := make([]Stock, 0, len(picks))
	for _, p := range picks {
		s := Stock{Name: p.Name, Ticker: p.Ticker, InvestingWindow: p.InvestingWindow,
			ExpectedMean: p.ExpectedMean, ExpectedVariance: p.ExpectedVariance,
			Confidence: p.Confidence, Comments: p.Comments}
		if fetched[p.SourcePostID] {
			s.TweetURL = "https://x.com/i/status/" + p.SourcePostID
		}
		stocks = append(stocks, s)
	}
	return stocks
}

func decodePicks(resp *anthropic.BetaMessage) ([]pick, error) {
	if resp.StopReason == anthropic.BetaStopReasonRefusal {
		return nil, fmt.Errorf("refused: %s", resp.StopDetails.Explanation)
	}
	if resp.StopReason == anthropic.BetaStopReasonMaxTokens {
		return nil, fmt.Errorf("reply truncated at max_tokens")
	}
	for _, block := range resp.Content {
		if t, ok := block.AsAny().(anthropic.BetaTextBlock); ok {
			var out struct{ Stocks []pick }
			return out.Stocks, json.Unmarshal([]byte(t.Text), &out)
		}
	}
	return nil, fmt.Errorf("no text block in reply")
}

func formatPosts(posts []Post) string {
	var b strings.Builder
	for _, p := range posts {
		fmt.Fprintf(&b, "<post id=%q at=%q likes=%d reposts=%d>\n%s\n</post>\n",
			p.ID, p.CreatedAt, p.PublicMetrics.Likes, p.PublicMetrics.Reposts, p.Text)
	}
	return b.String()
}
