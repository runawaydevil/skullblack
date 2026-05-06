package content

import (
	"context"
	"errors"
	"fmt"
	"html"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/parser"
	"github.com/microcosm-cc/bluemonday"

	"murad.world/murad-world/internal/baserow"
)

const markdownMaxBytes = 1 << 20 // 1MB

var markdownClient = &http.Client{Timeout: 10 * time.Second}

func prepareEntry(ctx context.Context, e baserow.Entry) (PreparedEntry, error) {
	mdFile, hasFile := resolveMarkdownFile(e)

	// Choose input text.
	var (
		inputText       string
		hasMarkdownUsed bool
		mdFileName      string
	)

	if hasFile {
		mdFileName = pickBaserowFileName(mdFile)
		txt, err := fetchMarkdownFile(ctx, mdFile.URL)
		if err == nil {
			inputText = txt
			hasMarkdownUsed = true
		} else {
			// Fetch failed; fall back to Body.
			inputText = strings.TrimSpace(e.Body)
		}
	} else {
		inputText = strings.TrimSpace(e.Body)
	}

	if strings.TrimSpace(inputText) == "" {
		inputText = "No content yet."
	}

	rendered, plain, preview := renderSanitizedMarkdown(inputText)
	return PreparedEntry{
		Entry:            e,
		RenderedHTML:     rendered,
		PlainText:        plain,
		PreviewText:      preview,
		HasMarkdownFile:  hasMarkdownUsed,
		MarkdownFileName: mdFileName,
	}, nil
}

func fallbackPreparedEntry(e baserow.Entry) PreparedEntry {
	inputText := strings.TrimSpace(e.Body)
	if inputText == "" {
		inputText = "No content yet."
	}
	rendered, plain, preview := renderSanitizedMarkdown(inputText)
	return PreparedEntry{
		Entry:        e,
		RenderedHTML: rendered,
		PlainText:    plain,
		PreviewText:  preview,
	}
}

func resolveMarkdownFile(e baserow.Entry) (baserow.BaserowFile, bool) {
	for _, f := range e.Markdown {
		if !isSafeHTTPURL(f.URL) {
			continue
		}
		if f.Size > 0 && f.Size > markdownMaxBytes {
			continue
		}
		if isMarkdownByName(f) || isMarkdownByMime(f) {
			return f, true
		}
	}
	return baserow.BaserowFile{}, false
}

func isMarkdownByName(f baserow.BaserowFile) bool {
	name := strings.ToLower(strings.TrimSpace(pickBaserowFileName(f)))
	return strings.HasSuffix(name, ".md") || strings.HasSuffix(name, ".markdown")
}

func isMarkdownByMime(f baserow.BaserowFile) bool {
	mt := strings.ToLower(strings.TrimSpace(f.MimeType))
	switch mt {
	case "text/markdown", "text/x-markdown", "text/plain":
		return true
	default:
		return false
	}
}

func pickBaserowFileName(f baserow.BaserowFile) string {
	if v := strings.TrimSpace(f.VisibleName); v != "" {
		return v
	}
	return strings.TrimSpace(f.Name)
}

func fetchMarkdownFile(ctx context.Context, rawURL string) (string, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return "", errors.New("empty markdown URL")
	}
	if !isSafeHTTPURL(rawURL) {
		return "", errors.New("unsafe markdown URL")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", fmt.Errorf("new request: %w", err)
	}

	resp, err := markdownClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch markdown: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("fetch markdown: unexpected status %d", resp.StatusCode)
	}

	// Enforce size limit even when the JSON doesn't include size.
	b, err := io.ReadAll(io.LimitReader(resp.Body, markdownMaxBytes+1))
	if err != nil {
		return "", fmt.Errorf("read markdown: %w", err)
	}
	if len(b) > markdownMaxBytes {
		return "", fmt.Errorf("markdown file too large (>%d bytes)", markdownMaxBytes)
	}

	// "Boring" normalization: keep as-is, just ensure valid string.
	return string(b), nil
}

func renderSanitizedMarkdown(input string) (template.HTML, string, string) {
	md := []byte(input)
	ext := parser.CommonExtensions | parser.AutoHeadingIDs
	p := parser.NewWithExtensions(ext)
	htmlBytes := markdown.ToHTML(md, p, nil)

	policy := bluemonday.UGCPolicy()
	policy.AllowURLSchemes("http", "https")
	policy.RequireNoFollowOnLinks(true)
	policy.RequireNoReferrerOnLinks(true)
	safe := policy.SanitizeBytes(htmlBytes)

	// Ensure rel includes noopener+noreferrer. (We keep it minimal and safe.)
	safe = []byte(ensureRelNoopenerNoreferrer(string(safe)))

	plain := strings.TrimSpace(stripTags(string(safe)))
	plain = html.UnescapeString(plain)
	plain = strings.Join(strings.Fields(plain), " ")

	preview := truncate(plain, 1200)
	return template.HTML(string(safe)), plain, preview
}

var (
	reAnchorOpen = regexp.MustCompile(`(?i)<a\s+[^>]*>`)
	reRelAttr    = regexp.MustCompile(`(?i)\srel="([^"]*)"`)
)

func ensureRelNoopenerNoreferrer(s string) string {
	return reAnchorOpen.ReplaceAllStringFunc(s, func(a string) string {
		if reRelAttr.MatchString(a) {
			return reRelAttr.ReplaceAllStringFunc(a, func(rel string) string {
				m := reRelAttr.FindStringSubmatch(rel)
				current := ""
				if len(m) == 2 {
					current = m[1]
				}
				tokens := splitRelTokens(current)
				tokens["noopener"] = true
				tokens["noreferrer"] = true
				return ` rel="` + joinRelTokens(tokens) + `"`
			})
		}
		// Insert rel right after "<a"
		return strings.Replace(a, "<a", `<a rel="noopener noreferrer"`, 1)
	})
}

func splitRelTokens(rel string) map[string]bool {
	out := make(map[string]bool)
	for _, t := range strings.Fields(strings.ToLower(rel)) {
		out[t] = true
	}
	return out
}

func joinRelTokens(tokens map[string]bool) string {
	// Stable-ish order; keep minimal set first.
	order := []string{"noopener", "noreferrer", "nofollow"}
	var out []string
	seen := make(map[string]bool)
	for _, k := range order {
		if tokens[k] {
			out = append(out, k)
			seen[k] = true
		}
	}
	for k := range tokens {
		if !seen[k] && tokens[k] {
			out = append(out, k)
		}
	}
	return strings.Join(out, " ")
}

func stripTags(s string) string {
	var b strings.Builder
	b.Grow(len(s))

	inTag := false
	for _, r := range s {
		switch r {
		case '<':
			inTag = true
		case '>':
			inTag = false
		default:
			if !inTag {
				b.WriteRune(r)
			}
		}
	}
	return b.String()
}

func truncate(s string, max int) string {
	s = strings.TrimSpace(s)
	if s == "" || max <= 0 {
		return ""
	}
	if len(s) <= max {
		return s
	}
	return strings.TrimSpace(s[:max]) + "…"
}

func isSafeHTTPURL(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil || u == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(u.Scheme)) {
	case "http", "https":
		return strings.TrimSpace(u.Host) != ""
	default:
		return false
	}
}
