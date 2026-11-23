package main

import (
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"sort"
	"time"
)

// maxItems bounds reads.xml so feed readers stay fast; oldest items fall off.
const maxItems = 500

type rssFeed struct {
	XMLName xml.Name   `xml:"rss"`
	Version string     `xml:"version,attr"`
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Title         string    `xml:"title"`
	Link          string    `xml:"link"`
	Description   string    `xml:"description"`
	LastBuildDate string    `xml:"lastBuildDate"`
	Items         []rssItem `xml:"item"`
}

type rssItem struct {
	Title       string   `xml:"title"`
	Link        string   `xml:"link"`
	GUID        rssGUID  `xml:"guid"`
	PubDate     string   `xml:"pubDate"`
	Categories  []string `xml:"category"`
	Description string   `xml:"description"`
}

type rssGUID struct {
	IsPermaLink bool   `xml:"isPermaLink,attr"`
	Value       string `xml:",chardata"`
}

func newItem(p readPick, post Post, link string) rssItem {
	pub, err := time.Parse(time.RFC3339, post.CreatedAt)
	if err != nil {
		pub = time.Now().UTC()
	}
	desc := fmt.Sprintf("%s\n\nVia @%s: %s", p.Summary, post.Author, post.URL())
	return rssItem{
		Title:       p.Title,
		Link:        link,
		GUID:        rssGUID{Value: link}, // same paper from two posts = one item
		PubDate:     pub.Format(time.RFC1123Z),
		Categories:  append([]string{p.Kind}, p.Topics...),
		Description: desc,
	}
}

func loadFeed(path string) (*rssFeed, error) {
	f := &rssFeed{Version: "2.0", Channel: rssChannel{
		Title:       "Reads",
		Link:        "https://x.com",
		Description: "Papers, research, books and projects surfaced from X",
	}}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return f, nil
	}
	if err != nil {
		return nil, err
	}
	return f, xml.Unmarshal(b, f)
}

// merge adds items whose link is not already in the feed, newest first, and
// returns how many were new. Existing items are never rewritten, so a feed
// reader never sees an old item change under it.
func (f *rssFeed) merge(items []rssItem, now time.Time) int {
	seen := map[string]bool{}
	for _, it := range f.Channel.Items {
		seen[it.GUID.Value] = true
	}
	added := 0
	for _, it := range items {
		if !seen[it.GUID.Value] {
			seen[it.GUID.Value] = true
			f.Channel.Items = append(f.Channel.Items, it)
			added++
		}
	}
	sort.SliceStable(f.Channel.Items, func(i, j int) bool {
		a, _ := time.Parse(time.RFC1123Z, f.Channel.Items[i].PubDate)
		b, _ := time.Parse(time.RFC1123Z, f.Channel.Items[j].PubDate)
		return a.After(b)
	})
	if len(f.Channel.Items) > maxItems {
		f.Channel.Items = f.Channel.Items[:maxItems]
	}
	f.Channel.LastBuildDate = now.Format(time.RFC1123Z)
	return added
}

// save writes via a temp file so a crash never leaves half a feed.
func (f *rssFeed) save(path string) error {
	b, err := xml.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append([]byte(xml.Header), append(b, '\n')...), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
