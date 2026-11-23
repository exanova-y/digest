package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"time"
)

// stockQuery: a bare "stocks" search returned ~95% copy-paste bot ads (the
// API's relevancy sort lacks x.com "Top" spam filtering). Requiring a cashtag
// and excluding "watchlist" (the bots' stock phrase) left 20/20 real posts.
// Retweets/replies are dropped: duplicates and chatter, each a read credit.
const stockQuery = `has:cashtags (stocks OR "bull market") lang:en -is:retweet -is:reply -watchlist`

const stockPrompt = `You read a batch of recent X posts (search results for market talk, plus posts from accounts the user follows) and pick out stocks that are notable right now: heavy discussion, news, earnings, unusual moves, or strong conviction from posters. Most posts from followed accounts will not be about stocks; ignore those.
For each stock:
- source_post_id: the id of the single post that best shows why it is notable (copy it exactly from the <post id=...> tag).
- investing_window: the horizon over which the catalyst in the posts is likely to play out (a momentum spike is short; a fundamental shift is longer).
- expected_mean: expected return over that investing_window, in percent (e.g. 2.5 means +2.5%).
- expected_variance: variance of that return over the same window, in percent squared (e.g. 64 means an 8% standard deviation).
Base estimates on the posts plus your general knowledge of the company's volatility. Skip crypto, indexes and ETFs, and skip tickers mentioned only in passing.
- confidence: how well the posts support these estimates. high = many independent posters and a concrete, verifiable catalyst; medium = clear catalyst but thin or one-sided discussion; low = hype, rumor, or a single source.
- comments: the cues behind the pick as comma-separated values, under 30 words in total, e.g. "CEO reinstated, +31% intraday, sector-wide semis rally, bears call relief rally".
Return an empty list if nothing qualifies.`

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

// stockPick is the model's row. It names a post id rather than a URL so the
// link is built in code from posts we actually fetched, never invented.
type stockPick struct {
	Name             string  `json:"name"`
	Ticker           string  `json:"ticker"`
	SourcePostID     string  `json:"source_post_id"`
	InvestingWindow  string  `json:"investing_window"`
	ExpectedMean     float64 `json:"expected_mean"`
	ExpectedVariance float64 `json:"expected_variance"`
	Confidence       string  `json:"confidence"`
	Comments         string  `json:"comments"`
}

func runStocks(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("stocks", flag.ExitOnError)
	o := bindFlags(fs, "top,following", stockQuery, "stocks.json", 6*time.Hour)
	fs.Parse(args)

	posts, err := gather(ctx, *o)
	if err != nil {
		return err
	}
	var reply struct {
		Stocks []stockPick `json:"stocks"`
	}
	if err := askClaude(ctx, stockPrompt, stockSchema, formatPosts(posts), &reply); err != nil {
		return fmt.Errorf("claude: %w", err)
	}
	ledger, err := loadLedger(o.Out)
	if err != nil {
		return err
	}
	ledger = mergeLedger(ledger, toStocks(reply.Stocks, posts), time.Now().UTC())
	if err := saveLedger(o.Out, ledger); err != nil {
		return err
	}
	fmt.Printf("stocks: %d posts -> %d notable; %s holds %d\n", len(posts), len(reply.Stocks), o.Out, len(ledger))
	return nil
}

// toStocks links each pick to its source post. A pick whose id is not among
// the fetched posts keeps an empty URL rather than a guessed one.
func toStocks(picks []stockPick, posts []Post) []Stock {
	byID := postsByID(posts)
	stocks := make([]Stock, 0, len(picks))
	for _, p := range picks {
		s := Stock{Name: p.Name, Ticker: p.Ticker, InvestingWindow: p.InvestingWindow,
			ExpectedMean: p.ExpectedMean, ExpectedVariance: p.ExpectedVariance,
			Confidence: p.Confidence, Comments: p.Comments}
		if post, ok := byID[p.SourcePostID]; ok {
			s.TweetURL = post.URL()
		}
		stocks = append(stocks, s)
	}
	return stocks
}
