package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go/v2"
)

// askClaude sends content under a system prompt and decodes the
// schema-constrained JSON reply into out. Schemas are raw JSON, not Go maps:
// maps marshal keys alphabetically, and with "comments" forced before
// "name"/"ticker" the model returned all-blank rows in 3 of 4 runs.
// Empty content is a no-op, leaving out at its zero value.
func askClaude(ctx context.Context, system string, schema json.RawMessage, content string, out any) error {
	if content == "" {
		return nil
	}
	client := anthropic.NewClient()
	resp, err := client.Beta.Messages.New(ctx, anthropic.BetaMessageNewParams{
		Model:     "claude-opus-5-5",
		MaxTokens: 16000,
		Betas:     []anthropic.AnthropicBeta{anthropic.AnthropicBetaServerSideFallback2026_07_01},
		Fallbacks: anthropic.BetaFallbacksParamOfDefault(),
		System:    []anthropic.BetaTextBlockParam{{Text: system}},
		OutputConfig: anthropic.BetaOutputConfigParam{
			Effort: anthropic.BetaOutputConfigEffortHigh,
			Format: anthropic.BetaJSONOutputFormatParam{Schema: schema},
		},
		Messages: []anthropic.BetaMessageParam{
			anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock(content)),
		},
	})
	if err != nil {
		return err
	}
	switch resp.StopReason {
	case anthropic.BetaStopReasonRefusal:
		return fmt.Errorf("refused: %s", resp.StopDetails.Explanation)
	case anthropic.BetaStopReasonMaxTokens:
		return fmt.Errorf("reply truncated at max_tokens")
	}
	for _, block := range resp.Content {
		if t, ok := block.AsAny().(anthropic.BetaTextBlock); ok {
			return json.Unmarshal([]byte(t.Text), out)
		}
	}
	return fmt.Errorf("no text block in reply")
}

// formatPosts tags each post with its id and links so the model can cite
// them; code later accepts only ids and URLs that appear here. No posts
// gives "", which askClaude treats as nothing to ask.
func formatPosts(posts []Post) string {
	var b strings.Builder
	for _, p := range posts {
		fmt.Fprintf(&b, "<post id=%q author=\"@%s\" at=%q likes=%d reposts=%d links=%q>\n%s\n</post>\n",
			p.ID, p.Author, p.CreatedAt, p.PublicMetrics.Likes, p.PublicMetrics.Reposts,
			strings.Join(p.Links(), " "), p.Text)
	}
	return b.String()
}
