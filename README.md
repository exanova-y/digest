# digest

Searches X for cashtag posts mentioning stocks or "bull market" from the last 6 hours (relevancy order), asks Claude (`claude-opus-5-5`) which stocks are notable, and merges them into `stocks.json`.

```
digest/
├── .env.example      # copy to .env (gitignored)
├── go.mod
├── hello-world.go
└── scan/
    ├── main.go       # flags + pipeline: env → fetch → extract → merge → save
    ├── env.go        # .env loader, hands X bearer token to xurl
    ├── search.go     # X recent search via xurl, decodes posts
    ├── extract.go    # one Claude structured-output call → []Stock
    └── ledger.go     # stocks.json load / merge by ticker / atomic save
```

## Current state

- One-shot scan works end to end against live X search (first run 2026-11-23: 100 posts → MRVL, MU, NVDA, SMCI, TSM).
- Default query: `has:cashtags (stocks OR "bull market") lang:en -is:retweet -is:reply -watchlist`. A bare `stocks` search came back ~95% copy-paste bot spam; the API's relevancy sort is not x.com's "Top" tab. Identical post texts are deduped before Claude.
- The Claude schema is ordered raw JSON on purpose: with keys in alphabetical order (`comments` first), structured output returned blank rows in 3 of 4 runs.
- `investing_window` is one of `1 day`, `3 days`, `1 week`, `2 weeks`, `1 month`, `3 months`, `6 months`, `1 year`, picked by Claude for how long the catalyst should play out. `expected_mean` (return in %) and `expected_variance` (%²) are over that window, estimated by Claude from the posts plus its prior on the company's volatility. They are opinions, not model fits.
- `confidence` is `low` / `medium` / `high`: how well the posts support the estimate (independent posters + concrete catalyst = high; hype or single source = low). `comments` is comma-separated cues, under 30 words.
- `tweet_url` is built in code from the post id Claude cites, and only if that id was in the fetched batch; otherwise it is empty.
- Merge rule: tickers seen this scan are overwritten and restamped. Tickers not seen keep their old row and timestamp.

## Output (`stocks.json`)

```json
[
  {
    "name": "NVIDIA",
    "ticker": "NVDA",
    "tweet_url": "https://x.com/i/status/2124913710334705877",
    "investing_window": "1 month",
    "expected_variance": 250,
    "expected_mean": 2,
    "confidence": "high",
    "comments": "Jensen Huang reinstated as CEO, +31-33% intraday, Trump statement backs board, semis sector-wide rally, bears call relief rally, rates repricing risk",
    "timestamp_last_edited": "2026-11-23T20:52:56Z"
  }
]
```

## Setup

1. `cp .env.example .env` and fill in `ANTHROPIC_API_KEY` and `X_BEARER_TOKEN`.
2. On each run, `scan` loads `.env` (shell vars win) and pipes the bearer token to `xurl auth app-only -`, because xurl only reads credentials from `~/.xurl`. OAuth1 is not used: xurl v1.3.4 mis-signs OAuth1 requests whose query contains a space (401).
3. X API is pay-per-use; each run reads up to `-n` posts. A 402 `credits depleted` means top up in the developer console.
4. `xurl` must be on `PATH` (`~/go/bin`).

## Run

```
go run ./scan                          # top 100 cashtag posts, last 6h → ./stocks.json
go run ./scan -since 2h -n 50
go run ./scan -q 'has:cashtags from:somefund' -out data/stocks.json
```
