// Command scan searches X for recent market talk, asks Claude which stocks are
// notable, and merges the result into stocks.json keyed by ticker.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"
)

// defaultQuery: a bare "stocks" search returned ~95% copy-paste bot ads (the
// API's relevancy sort lacks x.com "Top" spam filtering). Requiring a cashtag
// and excluding "watchlist" (the bots' stock phrase) left 20/20 real posts.
// Retweets/replies are dropped: duplicates and chatter, each a read credit.
const defaultQuery = `has:cashtags (stocks OR "bull market") lang:en -is:retweet -is:reply -watchlist`

func main() {
	out := flag.String("out", "stocks.json", "ledger file to merge into")
	query := flag.String("q", defaultQuery, "X search query")
	window := flag.Duration("since", 6*time.Hour, "how far back to search (max 7 days)")
	n := flag.Int("n", 100, "posts to read (10-100)")
	flag.Parse()

	if err := loadDotEnv(".env"); err != nil {
		log.Fatalf(".env: %v", err)
	}
	ctx := context.Background()
	if err := syncXurlAuth(ctx); err != nil {
		log.Fatalf("auth: %v", err)
	}
	posts, err := searchPosts(ctx, *query, *window, *n)
	if err != nil {
		log.Fatalf("search: %v", err)
	}
	found, err := extractStocks(ctx, posts)
	if err != nil {
		log.Fatalf("extract: %v", err)
	}
	ledger, err := loadLedger(*out)
	if err != nil {
		log.Fatalf("load: %v", err)
	}
	ledger = mergeLedger(ledger, found, time.Now().UTC())
	if err := saveLedger(*out, ledger); err != nil {
		log.Fatalf("save: %v", err)
	}
	fmt.Printf("%d posts -> %d notable stocks; %s now holds %d\n",
		len(posts), len(found), *out, len(ledger))
}
