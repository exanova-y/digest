package main

import (
	"context"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"
)

// page is the text of a linked source, for embedding.
type page struct {
	URL   string
	Title string
	Text  string // visible text, tags stripped, capped
}

var (
	reHidden = regexp.MustCompile(`(?is)<(script|style|noscript|nav|header|footer)\b.*?</(script|style|noscript|nav|header|footer)>`)
	reTag    = regexp.MustCompile(`(?s)<[^>]+>`)
	reSpace  = regexp.MustCompile(`\s+`)
	reTitle  = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
)

// maxText bounds what is embedded: text-embedding-3-small reads at most
// 8191 tokens (~4 chars each), and the opening of a page carries its voice.
const maxText = 20000

var httpClient = &http.Client{Timeout: 20 * time.Second}

// fetchPage downloads an HTML page. arXiv abstract and PDF links are swapped
// for the HTML rendering, which carries the figures and full text.
func fetchPage(ctx context.Context, link string) (page, error) {
	link = arxivHTML(link)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, nil)
	if err != nil {
		return page{}, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; digest-reads/1.0; personal feed)")
	resp, err := httpClient.Do(req)
	if err != nil {
		return page{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return page{}, fmt.Errorf("%s: HTTP %d", link, resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "html") {
		return page{}, fmt.Errorf("%s: not HTML (%s)", link, ct)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 5<<20))
	if err != nil {
		return page{}, err
	}
	return parsePage(link, string(body)), nil
}

func parsePage(link, raw string) page {
	p := page{URL: link}
	if m := reTitle.FindStringSubmatch(raw); m != nil {
		p.Title = clean(m[1])
	}
	p.Text = clean(reHidden.ReplaceAllString(raw, " "))
	if len(p.Text) > maxText {
		p.Text = p.Text[:maxText]
	}
	return p
}

// postPage stands in for a source page that will not load (paywall, 403):
// the post and Claude's summary are embedded instead, so a blocked site
// lowers confidence in the score rather than silently dropping the item.
// Keyed by post URL so it never shadows the real page in the cache.
func postPage(p readPick, post Post) page {
	return page{URL: post.URL(), Title: p.Title, Text: p.Summary + "\n\n" + post.Text}
}

func arxivHTML(link string) string {
	u, err := url.Parse(link)
	if err != nil || !strings.HasSuffix(u.Host, "arxiv.org") {
		return link
	}
	for _, prefix := range []string{"/abs/", "/pdf/"} {
		if id, ok := strings.CutPrefix(u.Path, prefix); ok {
			return "https://arxiv.org/html/" + strings.TrimSuffix(id, ".pdf")
		}
	}
	return link
}

func clean(s string) string {
	return strings.TrimSpace(reSpace.ReplaceAllString(html.UnescapeString(reTag.ReplaceAllString(s, " ")), " "))
}

func trim(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

type candidate struct {
	pick readPick
	post Post
	page page
}

// screen enforces the reads rules in code, logging why each pick fails:
// the source page must be linked from its post (never a guessed URL) and sit
// within minSim of at least one anchor text. A "has its own graphs" rule was
// tried and dropped: it rejected picks the user wanted (NIST metalenses,
// a NASA space-structures write-up) and passed Amazon book listings.
func screen(ctx context.Context, picks []readPick, posts []Post, anchorsPath string, minSim float64) ([]rssItem, error) {
	if len(picks) == 0 {
		return nil, nil
	}
	emb, err := newEmbedder()
	if err != nil {
		return nil, err
	}
	anchorURLs, anchorVecs, err := anchorVectors(ctx, emb, anchorsPath)
	if err != nil {
		return nil, err
	}
	byID := postsByID(posts)
	var cands []candidate
	for _, p := range picks {
		post, ok := byID[p.SourcePostID]
		switch {
		case !ok:
			logPick("drop", p, "cited post not in batch")
		case !slices.Contains(post.Links(), p.Link):
			logPick("drop", p, "no source link in post")
		default:
			pg, err := fetchPage(ctx, p.Link)
			if err != nil {
				fmt.Printf("  note  %-50s  source unreadable, scoring the post instead: %v\n", trim(p.Title, 50), err)
				pg = postPage(p, post)
			}
			cands = append(cands, candidate{p, post, pg})
		}
	}
	pages := make([]page, len(cands))
	for i, c := range cands {
		pages[i] = c.page
	}
	vecs, err := emb.vectors(ctx, pages)
	if err != nil {
		return nil, err
	}
	var items []rssItem
	for i, c := range cands {
		sim, at := nearest(vecs[i], anchorVecs)
		why := fmt.Sprintf("sim %.3f to %s", sim, anchorURLs[at])
		if sim < minSim {
			logPick("drop", c.pick, why)
			continue
		}
		logPick("keep", c.pick, why)
		items = append(items, newItem(c.pick, c.post, c.pick.Link))
	}
	return items, nil
}

// anchorVectors fetches only anchors missing from the embedding cache.
func anchorVectors(ctx context.Context, emb *embedder, path string) ([]string, [][]float64, error) {
	urls, err := readAnchors(path)
	if err != nil {
		return nil, nil, fmt.Errorf("anchors: %w", err)
	}
	var keep []string
	var pages []page
	for _, u := range urls {
		pg := page{URL: u}
		if _, cached := emb.cache.Vectors[u]; !cached {
			if pg, err = fetchPage(ctx, u); err != nil {
				fmt.Printf("  anchor skipped: %v\n", err)
				continue
			}
			pg.URL = u // cache under the listed URL, not the arXiv HTML rewrite
		}
		keep, pages = append(keep, u), append(pages, pg)
	}
	if len(pages) == 0 {
		return nil, nil, fmt.Errorf("no readable anchors in %s", path)
	}
	vecs, err := emb.vectors(ctx, pages)
	return keep, vecs, err
}

func logPick(verdict string, p readPick, why string) {
	fmt.Printf("  %s  %-50s  %s\n", verdict, trim(p.Title, 50), why)
}
