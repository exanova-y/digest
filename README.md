# digest

Reads recent X posts from up to three sources, **top** (X search, relevancy order), **following** (your home timeline) and **accounts** (named institutions, reads only), and has Claude (`claude-opus-5-5`) distill them into:

- `scan stocks`: notable stocks → `stocks.json`
- `scan reads`: papers, research, books and projects → `reads.xml` (RSS 2.0, for a personal feed reader)

```
digest/
├── .github/workflows/reads.yml  # weekday 08:00 Los Angeles schedule
├── .env.example      # copy to .env (gitignored)
├── anchors.txt       # reference essays (URLs) for the reads worldview check
├── CHANGELOG.md
├── go.mod
├── hello-world.go
├── stocks.json       # output of `scan stocks`
├── reads.xml         # output of `scan reads`
└── scan/
    ├── main.go       # subcommand dispatch (stocks | reads)
    ├── env.go        # .env loader; hands X credentials to xurl
    ├── source.go     # X sources (top, following, accounts), Post, shared flags
    ├── claude.go     # one structured-output Claude call, shared by both commands
    ├── stocks.go     # stocks prompt/schema → []Stock
    ├── ledger.go     # stocks.json load / merge by ticker / atomic save
    ├── reads.go      # reads prompt/schema/query, command flow
    ├── vet.go        # reads screening: fetch source page, similarity gate
    ├── embed.go      # embeddings via OpenRouter, URL-keyed cache, cosine to nearest anchor
    └── feed.go       # reads.xml RSS load / merge by link / atomic save
```

## Current state

- Both commands work end to end against live X (2026-11-23). First `reads` run: 195 posts → 10 items (arXiv preprints, research posts, projects, books).
- Sources: `-source` takes a comma list of `top`, `following`, `accounts` (`both` = top,following). Defaults: stocks `top,following`; reads all three. Each source costs up to `-n` read credits. Posts seen in two sources are deduped by id, and identical texts are deduped to drop spam bots.
- Links are never invented. Claude cites a post id; `tweet_url` / the RSS `<link>` are built in code from fetched posts. A reads item links to the paper/repo only if that exact URL is in its post, otherwise to the post.
- Claude schemas are ordered raw JSON on purpose: with keys in alphabetical order (`comments` first), structured output returned blank rows in 3 of 4 runs.

### stocks
- Search query: `has:cashtags (stocks OR "bull market") lang:en -is:retweet -is:reply -watchlist`. A bare `stocks` search came back ~95% copy-paste bot spam; the API's relevancy sort is not x.com's "Top" tab.
- `investing_window` is one of `1 day` … `1 year`, picked by Claude for how long the catalyst should play out. `expected_mean` (return in %) and `expected_variance` (%²) are over that window. These are Claude's opinions, not model fits.
- `confidence` is `low` / `medium` / `high`: how well the posts support the estimate. `comments` is comma-separated cues, under 30 words.
- Merge rule: tickers seen this run are overwritten and restamped; others keep their old row.

### reads
- `-include` (default `applied physics,lab instrumentation,ultrasound,engineering,neurotech`) and `-exclude` (default `AI safety,crypto`) build both the X search query and Claude's filter. The following timeline can't be filtered server-side, so Claude applies the terms there.
- `accounts` source: `-accounts` (default `Caltech,MIT,maxplanckpress`, the official handles, checked via X user lookup) over `-accounts-since` (default 168h, the search API max). These institutions post about once a day, so the 6h window would usually be empty. First run: 12 posts in 7 days → 2 MIT News items (primordial black holes, gel-free ultrasound).
- Each item has `kind` (paper / research / book / project) and its matched topics as `<category>` tags, a summary, and "Via @author" with the source post.
- Merge rule: new items are added by link (the same paper from two posts appears once), newest first, capped at 500. Existing items are never rewritten.
- Filter rule ("high-resolution worldview, no faked results"), applied in two stages:
  1. Claude picks candidates. The prompt states the bar and the topic terms, and tells it to skip unverifiable or exaggerated results and book promos.
  2. Code checks each candidate: its post must link the source. The source page's **embedding** (`text-embedding-3-small`, 512 dims via OpenRouter, the same model as the garden's curius recommender) must be within cosine `-min-sim` of the nearest text in `anchors.txt` (Mead, Roughgarden, Yunger Halpern, Cosmos Institute). arXiv links are read via `arxiv.org/html/…`. If the page won't load (paywall, 403), the post and Claude's summary are scored instead. Each kept or dropped candidate is printed with the reason and its similarity score.
- "No faked results" is only checked by Claude in stage 1; embeddings cannot detect it.
- A "has its own graphs" check was tried and removed: it rejected picks the user wanted (NIST metalenses, a NASA space-structures write-up) and passed Amazon book listings.
- `-min-sim` defaults to 0.42, calibrated 2026-11-23. Anchors score 0.52–0.76 against each other (same author ~0.71–0.76). Of the 11 readable pre-filter feed items, the physics/ultrasound research pieces scored 0.51–0.66. The NIST metalenses piece, which the user wanted kept, scored 0.44. Book promos, a CV paper and a GPU benchmark scored ≤ 0.40. The margin is narrow and the sample small, so re-check as the feed grows.
- Default window for reads is `-since 24h` (stocks stays 6h).
- Similarity also tracks topic, not just voice: most items' nearest anchor is a Quantum Frontiers physics post.
- Embeddings are cached by URL in `.embed-cache.json` (gitignored); delete it to re-embed.

## Output

`stocks.json`
```json
{
  "name": "NVIDIA",
  "ticker": "NVDA",
  "tweet_url": "https://x.com/i/status/2124913710334705877",
  "investing_window": "1 week",
  "expected_variance": 250,
  "expected_mean": 2,
  "confidence": "high",
  "comments": "Jensen Huang reinstated CEO, +31-33% intraday, ~$1T cap added, Trump statement, bears call relief rally",
  "timestamp_last_edited": "2026-11-23T20:57:30Z"
}
```

`reads.xml` item
```xml
<item>
  <title>Multiple-Camera Tracking of Rigid Objects</title>
  <link>https://arxiv.org/abs/2611.25904</link>
  <guid isPermaLink="false">https://arxiv.org/abs/2611.25904</guid>
  <pubDate>Mon, 23 Nov 2026 20:48:52 +0000</pubDate>
  <category>paper</category>
  <category>engineering</category>
  <description>Robotics and computer-vision preprint … Via @OWW: https://x.com/i/status/2124968446752383102</description>
</item>
```

## Setup

1. `cp .env.example .env` and fill in:
   - `ANTHROPIC_API_KEY`
   - `X_BEARER_TOKEN` for the top source (app-only search)
   - `X_CONSUMER_KEY`, `X_CONSUMER_SECRET`, `X_ACCESS_TOKEN`, `X_ACCESS_TOKEN_SECRET` for the following source (the home timeline needs user context)
   - `OPENROUTER_API_KEY` for `scan reads` (embeddings; ~$0.02 per million tokens)
2. On each run, `scan` loads `.env` (shell vars win) and registers the credentials with xurl, which only reads `~/.xurl`. Search uses the bearer token because xurl v1.3.4 mis-signs OAuth1 requests whose query contains a space (401). The timeline path has no spaces, so OAuth1 works there.
3. X API is pay-per-use; each run reads up to `-n` posts per source. A 402 `credits depleted` means top up in the developer console.
4. `xurl` must be on `PATH` (`~/go/bin`).

## Run

```
go run ./scan stocks                     # both sources, last 6h → ./stocks.json
go run ./scan reads                      # both sources, last 6h → ./reads.xml
go run ./scan reads -source following    # only accounts you follow
go run ./scan reads -source accounts -accounts 'Caltech,MIT,maxplanckpress,MPI_Quantum'
go run ./scan reads -include 'neurotech,ultrasound,BCI' -exclude 'crypto' -since 24h
go run ./scan reads -min-sim 0.6 -anchors anchors.txt
go run ./scan stocks -source top -q 'has:cashtags from:somefund'
```
`go run ./scan` with no subcommand still runs `stocks`. Shared flags: `-source`, `-since`, `-n`, `-q`, `-out`.

## Schedule

`.github/workflows/reads.yml` runs `scan reads` on GitHub Actions at **08:00 America/Los_Angeles on weekdays** and commits `reads.xml` back to the repo when it gains items.
- The feed's public URL is `https://raw.githubusercontent.com/exanova-y/digest/main/reads.xml`.
- GitHub cron is UTC with no DST, so 15:00 and 16:00 UTC are both scheduled and the workflow keeps whichever is 08:00 local that day. GitHub can start scheduled runs late (often 5–30 min).
- Credentials are repo secrets with the same names as in `.env`: `ANTHROPIC_API_KEY`, `OPENROUTER_API_KEY`, `X_BEARER_TOKEN`, `X_CONSUMER_KEY`, `X_CONSUMER_SECRET`, `X_ACCESS_TOKEN`, `X_ACCESS_TOKEN_SECRET`.
- The embedding cache is kept between runs with `actions/cache`.
- Run it by hand from the Actions tab ("Run workflow") or with `gh workflow run reads`.
- Cost per run is up to ~200 X post reads (≈ $0.70 at today's observed ~$0.003/read), one Claude call and a few cents of embeddings.
