package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"strings"
	"time"
)

const readsPrompt = `You read a batch of recent X posts (search results plus posts from accounts the user follows) and pick out interesting papers, research results, books, or projects worth reading or trying later.
Include only items about: %s.
Exclude anything about: %s, even if it also touches an included topic.
The bar is high: work that gives a high-resolution worldview, explaining mechanisms and magnitudes from first principles, with theory tied to measurement or a built thing. Think Carver Mead, Nicole Yunger Halpern, Tim Roughgarden, the Cosmos Institute.
Only pick posts that link to the item's own source (paper, preprint, lab or author write-up, project page); the source page is checked separately for its own figures.
Skip anything whose results look faked, exaggerated or unverifiable: claims without methods or data, implausible numbers, AI-generated summaries, engagement bait.
Skip news, opinion threads, memes, product ads, book promos, job posts, and posts with nothing concrete to read or try.
For each item:
- source_post_id: the id of the post it came from (copy it exactly from the <post id=...> tag).
- link: the URL of the item itself, copied exactly from that post's links attribute; empty string if the post has no such link.
- kind: paper, research, book, or project.
- title: the item's own title if the post gives it, else a short descriptive title under 12 words.
- topics: which of the included topics it matches.
- summary: under 40 words on what it is and why it is interesting.
Return an empty list if nothing qualifies.`

// readsSchema is built per run so the topics enum matches -include exactly.
func readsSchema(include []string) json.RawMessage {
	topics, _ := json.Marshal(include)
	return json.RawMessage(fmt.Sprintf(`{
  "type": "object",
  "properties": {
    "reads": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "source_post_id": {"type": "string"},
          "link": {"type": "string"},
          "kind": {"type": "string", "enum": ["paper", "research", "book", "project"]},
          "title": {"type": "string"},
          "topics": {"type": "array", "items": {"type": "string", "enum": %s}},
          "summary": {"type": "string"}
        },
        "required": ["source_post_id", "link", "kind", "title", "topics", "summary"],
        "additionalProperties": false
      }
    }
  },
  "required": ["reads"],
  "additionalProperties": false
}`, topics))
}

type readPick struct {
	SourcePostID string   `json:"source_post_id"`
	Link         string   `json:"link"`
	Kind         string   `json:"kind"`
	Title        string   `json:"title"`
	Topics       []string `json:"topics"`
	Summary      string   `json:"summary"`
}

func runReads(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("reads", flag.ExitOnError)
	include := fs.String("include", "applied physics,lab instrumentation,ultrasound,engineering,neurotech", "comma-separated topics to keep")
	exclude := fs.String("exclude", "AI safety,crypto", "comma-separated topics to drop")
	o := bindFlags(fs, "top,following,accounts", "", "reads.xml", 24*time.Hour)
	fs.StringVar(&o.Accounts, "accounts", "Caltech,MIT,maxplanckpress", "X handles for the accounts source")
	// 7 days is the search API's limit; the feed dedupes by link, so
	// re-reading the same week each run adds nothing twice.
	fs.DurationVar(&o.AccountsWindow, "accounts-since", 7*24*time.Hour, "how far back to read the accounts source")
	anchors := fs.String("anchors", "anchors.txt", "reference texts (URLs) for the worldview check")
	minSim := fs.Float64("min-sim", 0.42, "minimum cosine similarity to the nearest anchor (calibrated 2026-11-23)")
	fs.Parse(args)

	inc, exc := splitTerms(*include), splitTerms(*exclude)
	if o.Query == "" {
		o.Query = readsQuery(inc, exc)
	}
	posts, err := gather(ctx, *o)
	if err != nil {
		return err
	}
	var reply struct {
		Reads []readPick `json:"reads"`
	}
	prompt := fmt.Sprintf(readsPrompt, strings.Join(inc, ", "), strings.Join(exc, ", "))
	if err := askClaude(ctx, prompt, readsSchema(inc), formatPosts(posts), &reply); err != nil {
		return fmt.Errorf("claude: %w", err)
	}
	items, err := screen(ctx, reply.Reads, posts, *anchors, *minSim)
	if err != nil {
		return err
	}
	feed, err := loadFeed(o.Out)
	if err != nil {
		return err
	}
	added := feed.merge(items, time.Now().UTC())
	if err := feed.save(o.Out); err != nil {
		return err
	}
	fmt.Printf("reads: %d posts -> %d picks -> %d passed, %d new; %s holds %d\n",
		len(posts), len(reply.Reads), len(items), added, o.Out, len(feed.Channel.Items))
	return nil
}

// readsQuery needs a link so posts point at something to read; the exclude
// terms are also filtered server-side to save read credits.
func readsQuery(include, exclude []string) string {
	q := "(" + strings.Join(quoteAll(include), " OR ") + ") (paper OR arxiv OR preprint OR research OR book OR project OR github) has:links lang:en -is:retweet -is:reply"
	for _, t := range quoteAll(exclude) {
		q += " -" + t
	}
	return q
}

func splitTerms(s string) []string {
	var out []string
	for _, t := range strings.Split(s, ",") {
		if t = strings.TrimSpace(t); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func quoteAll(terms []string) []string {
	out := make([]string, len(terms))
	for i, t := range terms {
		if strings.Contains(t, " ") {
			t = `"` + t + `"`
		}
		out[i] = t
	}
	return out
}
