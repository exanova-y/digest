package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/url"
	"os/exec"
	"strings"
	"time"
)

// Post is the subset of an X post that the extractors need.
type Post struct {
	ID            string `json:"id"`
	Text          string `json:"text"`
	CreatedAt     string `json:"created_at"`
	AuthorID      string `json:"author_id"`
	PublicMetrics struct {
		Likes   int `json:"like_count"`
		Reposts int `json:"retweet_count"`
	} `json:"public_metrics"`
	Entities struct {
		URLs []struct {
			Expanded string `json:"expanded_url"`
		} `json:"urls"`
	} `json:"entities"`
	NoteTweet struct {
		Text string `json:"text"`
	} `json:"note_tweet"`
	Author string `json:"-"` // username, filled from includes.users
}

func (p Post) URL() string { return "https://x.com/i/status/" + p.ID }

// Links are the post's outbound URLs. x.com links are dropped: they are the
// post's own media or quoted posts, never the paper or project itself.
func (p Post) Links() []string {
	var out []string
	for _, u := range p.Entities.URLs {
		parsed, err := url.Parse(u.Expanded)
		if err != nil || parsed.Host == "x.com" || parsed.Host == "twitter.com" {
			continue
		}
		out = append(out, u.Expanded)
	}
	return out
}

// gatherOpts are the flags every subcommand shares. Accounts is only bound
// by subcommands that offer the accounts source.
type gatherOpts struct {
	Source         string
	Query          string
	Window         time.Duration
	N              int
	Out            string
	Accounts       string
	AccountsWindow time.Duration
}

func bindFlags(fs *flag.FlagSet, sources, query, out string, since time.Duration) *gatherOpts {
	o := &gatherOpts{}
	fs.StringVar(&o.Source, "source", sources, "comma-separated: top (X search), following (your home timeline), accounts (-accounts handles); both = top,following")
	fs.StringVar(&o.Query, "q", query, "X search query for the top source")
	fs.DurationVar(&o.Window, "since", since, "how far back to read (search max 7 days)")
	fs.IntVar(&o.N, "n", 100, "posts per source (10-100)")
	fs.StringVar(&o.Out, "out", out, "output file")
	return o
}

const postFields = "tweet.fields=created_at,public_metrics,entities,note_tweet,author_id&expansions=author_id&user.fields=username"

// gather reads posts from the chosen sources. Search (top, accounts) uses
// app-only auth; the home timeline needs user context (OAuth1).
func gather(ctx context.Context, o gatherOpts) ([]Post, error) {
	src, err := parseSources(o.Source)
	if err != nil {
		return nil, err
	}
	if err := syncXurlAuth(ctx, src["top"] || src["accounts"], src["following"]); err != nil {
		return nil, err
	}
	start := time.Now().UTC().Add(-o.Window).Format(time.RFC3339)
	var posts []Post
	if src["top"] {
		got, err := searchPosts(ctx, o.Query, start, o.N)
		if err != nil {
			return nil, fmt.Errorf("top: %w", err)
		}
		posts = append(posts, got...)
	}
	if src["accounts"] {
		got, err := accountPosts(ctx, o)
		if err != nil {
			return nil, fmt.Errorf("accounts: %w", err)
		}
		posts = append(posts, got...)
	}
	if src["following"] {
		got, err := followingPosts(ctx, start, o.N)
		if err != nil {
			return nil, fmt.Errorf("following: %w", err)
		}
		posts = append(posts, got...)
	}
	return dedupe(posts), nil
}

func parseSources(s string) (map[string]bool, error) {
	src := map[string]bool{}
	for _, name := range splitTerms(s) {
		switch name {
		case "both":
			src["top"], src["following"] = true, true
		case "top", "following", "accounts":
			src[name] = true
		default:
			return nil, fmt.Errorf("unknown -source %q (want top, following, accounts)", name)
		}
	}
	if len(src) == 0 {
		return nil, fmt.Errorf("-source is empty")
	}
	return src, nil
}

func searchPosts(ctx context.Context, query, start string, n int) ([]Post, error) {
	q := url.Values{"query": {query}, "start_time": {start}, "sort_order": {"relevancy"}, "max_results": {fmt.Sprint(n)}}
	return xurlPosts(ctx, "app", "/2/tweets/search/recent?"+q.Encode()+"&"+postFields)
}

// accountPosts reads institutions' own posts over a longer window: they post
// about once a day (Caltech+MIT+Max Planck: 1 post in 24h when measured),
// so the 6h window used for top/following would usually be empty.
func accountPosts(ctx context.Context, o gatherOpts) ([]Post, error) {
	handles := splitTerms(o.Accounts)
	if len(handles) == 0 {
		return nil, fmt.Errorf("-accounts is empty")
	}
	for i, h := range handles {
		handles[i] = "from:" + strings.TrimPrefix(h, "@")
	}
	query := "(" + strings.Join(handles, " OR ") + ") -is:retweet -is:reply"
	start := time.Now().UTC().Add(-o.AccountsWindow).Format(time.RFC3339)
	return searchPosts(ctx, query, start, o.N)
}

// followingPosts reads the user's reverse-chronological home timeline. The
// path carries no spaces, which is what trips xurl's OAuth1 signing.
func followingPosts(ctx context.Context, start string, n int) ([]Post, error) {
	body, err := xurl(ctx, "oauth1", "/2/users/me")
	if err != nil {
		return nil, err
	}
	var me struct {
		Data struct{ ID string } `json:"data"`
	}
	if err := json.Unmarshal(body, &me); err != nil || me.Data.ID == "" {
		return nil, fmt.Errorf("resolve user id: %s", body)
	}
	q := url.Values{"start_time": {start}, "max_results": {fmt.Sprint(n)}}
	return xurlPosts(ctx, "oauth1", "/2/users/"+me.Data.ID+"/timelines/reverse_chronological?"+q.Encode()+"&"+postFields)
}

func xurl(ctx context.Context, auth, path string) ([]byte, error) {
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "xurl", "--auth", auth, path)
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	// xurl prints X API error bodies (e.g. 402 credits depleted) to stdout.
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("xurl: %w: %s%s", err, stderr.String(), stdout.String())
	}
	return stdout.Bytes(), nil
}

func xurlPosts(ctx context.Context, auth, path string) ([]Post, error) {
	body, err := xurl(ctx, auth, path)
	if err != nil {
		return nil, err
	}
	return decodePosts(body)
}

func decodePosts(body []byte) ([]Post, error) {
	var resp struct {
		Data     []Post `json:"data"`
		Includes struct {
			Users []struct{ ID, Username string } `json:"users"`
		} `json:"includes"`
		Errors []json.RawMessage `json:"errors"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode xurl output: %w", err)
	}
	if len(resp.Data) == 0 && len(resp.Errors) > 0 {
		return nil, fmt.Errorf("X API: %s", resp.Errors[0])
	}
	names := map[string]string{}
	for _, u := range resp.Includes.Users {
		names[u.ID] = u.Username
	}
	for i := range resp.Data {
		resp.Data[i].Author = names[resp.Data[i].AuthorID]
		if resp.Data[i].NoteTweet.Text != "" { // long posts are truncated in text
			resp.Data[i].Text = resp.Data[i].NoteTweet.Text
		}
	}
	return resp.Data, nil
}

// dedupe drops repeated ids (a post can be in both sources) and repeated
// text; spam bots repost verbatim and would otherwise read as consensus.
func dedupe(posts []Post) []Post {
	seen := map[string]bool{}
	out := posts[:0]
	for _, p := range posts {
		if !seen[p.ID] && !seen[p.Text] {
			seen[p.ID], seen[p.Text] = true, true
			out = append(out, p)
		}
	}
	return out
}

func postsByID(posts []Post) map[string]Post {
	m := make(map[string]Post, len(posts))
	for _, p := range posts {
		m[p.ID] = p
	}
	return m
}
