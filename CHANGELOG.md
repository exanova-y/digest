# Changelog

## 2026-11-23

- **schedule**: GitHub Actions runs `scan reads` weekdays 08:00 Los Angeles and commits `reads.xml`. Reads default window is now 24h.
- **reads filter**: graphs rule removed (rejected wanted picks). `-min-sim` 0.5 → 0.42. Unreadable source pages fall back to scoring the post. Candidates must link their source, and it must be similar to `anchors.txt` (`text-embedding-3-small` via OpenRouter, cosine ≥ `-min-sim`). Claude's prompt now states the worldview bar and the no-faked-results rule. Replaced an LLM scoring pass with embeddings, per the garden's curius recommender approach.

- **accounts source** (reads): posts from @Caltech, @MIT and @maxplanckpress over a 7-day window (`-accounts`, `-accounts-since`). `-source` is now a comma list.

- **scan reads** (new): papers / research / books / projects from X → `reads.xml` (RSS 2.0). Include/exclude terms drive both the search query and Claude's filter. Items link to the primary source only when that URL is in the post.
- **Sources**: both commands read `top` (X search), `following` (home timeline, OAuth1), or `both` (default).
- **scan stocks**: added `tweet_url`, `investing_window` (mean and variance are now over this window, no longer a fixed 30 days), `confidence`, and comma-separated cue `comments`.
- Restructured `scan/` into `source.go` (X), `claude.go` (model call), and one file per command (`stocks.go`, `reads.go`), plus their stores (`ledger.go`, `feed.go`).
- Fixed: structured output returned blank rows when the Go map schema marshalled keys alphabetically. Schemas are now ordered raw JSON.
- Fixed: home timeline → X search (the timeline had little market talk). The default query requires a cashtag and excludes "watchlist" bot spam.
- Workaround: xurl v1.3.4 OAuth1 signing fails on queries with spaces, so search uses an app-only bearer token.
