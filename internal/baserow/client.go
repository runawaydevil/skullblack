package baserow

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"
)

const maxErrBodyBytes = 2048

// Client fetches rows from the Baserow REST API.
type Client struct {
	baseURL    string
	tableID    string
	token      string
	httpClient *http.Client
}

// NewClient creates a Baserow API client.
func NewClient(baseURL, tableID, token string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		tableID: tableID,
		token:   token,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

type selectField struct {
	Value string `json:"value"`
}

type tagField struct {
	Value string `json:"value"`
}

type BaserowFile struct {
	URL         string                      `json:"url"`
	VisibleName string                      `json:"visible_name"`
	Name        string                      `json:"name"`
	Size        int64                       `json:"size"`
	MimeType    string                      `json:"mime_type"`
	IsImage     bool                        `json:"is_image"`
	UploadedAt  string                      `json:"uploaded_at"`
	ImageWidth  int                         `json:"image_width"`
	ImageHeight int                         `json:"image_height"`
	Thumbnails  map[string]BaserowThumbnail `json:"thumbnails"`
}

type BaserowThumbnail struct {
	URL    string `json:"url"`
	Width  *int   `json:"width"`
	Height *int   `json:"height"`
}

type rowDTO struct {
	ID           int           `json:"id"`
	Title        string        `json:"Title"`
	Slug         string        `json:"Slug"`
	Body         string        `json:"Body"`
	Summary      string        `json:"Summary"`
	SourceURL    string        `json:"Source URL"`
	SourceTitle  string        `json:"Source Title"`
	ImageURL     string        `json:"Image URL"`
	Image        []BaserowFile `json:"Image"`
	Markdown     []BaserowFile `json:"Markdown"`
	PublishedRaw string        `json:"Published at"`
	CreatedRaw   string        `json:"Created at"`
	UpdatedRaw   string        `json:"Updated at"`
	Type         *selectField  `json:"Type"`
	Status       *selectField  `json:"Status"`
	Visibility   *selectField  `json:"Visibility"`
	Spaces       []tagField    `json:"Spaces"`
	Tags         []tagField    `json:"Tags"`
}

type listResponse struct {
	Next    string   `json:"next"`
	Results []rowDTO `json:"results"`
}

// FetchPublicPublished returns entries visible on the public site, sorted by Published at desc.
func (c *Client) FetchPublicPublished(ctx context.Context) ([]Entry, error) {
	firstURL := fmt.Sprintf("%s/api/database/rows/table/%s/?user_field_names=true", c.baseURL, c.tableID)

	var all []rowDTO
	next := firstURL
	for next != "" {
		rows, nxt, err := c.fetchPage(ctx, next)
		if err != nil {
			return nil, err
		}
		all = append(all, rows...)
		next = nxt
	}

	var out []Entry
	for _, r := range all {
		if r.Status == nil || !strings.EqualFold(strings.TrimSpace(r.Status.Value), "published") {
			continue
		}
		if r.Visibility == nil || !strings.EqualFold(strings.TrimSpace(r.Visibility.Value), "public") {
			continue
		}
		e, ok := dtoToEntry(r)
		if !ok {
			continue
		}
		out = append(out, e)
	}

	sort.SliceStable(out, func(i, j int) bool {
		ti, tj := out[i].PublishedAt, out[j].PublishedAt
		if ti.Equal(tj) {
			return out[i].ID > out[j].ID
		}
		return ti.After(tj)
	})

	return out, nil
}

func (c *Client) fetchPage(ctx context.Context, pageURL string) ([]rowDTO, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Authorization", "Token "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, "", err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet := string(body)
		if len(snippet) > maxErrBodyBytes {
			snippet = snippet[:maxErrBodyBytes] + "…"
		}
		log.Printf("baserow: request failed status=%d body_prefix=%q", resp.StatusCode, snippet)
		return nil, "", fmt.Errorf("baserow: unexpected status %d", resp.StatusCode)
	}

	var lr listResponse
	if err := json.Unmarshal(body, &lr); err != nil {
		return nil, "", fmt.Errorf("baserow: decode response: %w", err)
	}

	return lr.Results, lr.Next, nil
}

func dtoToEntry(r rowDTO) (Entry, bool) {
	slug := strings.TrimSpace(r.Slug)
	if slug == "" {
		return Entry{}, false
	}
	var pub time.Time
	if t, ok := parseDate(r.PublishedRaw); ok {
		pub = t
	}
	var created, updated time.Time
	if t, ok := parseDate(r.CreatedRaw); ok {
		created = t
	}
	if t, ok := parseDate(r.UpdatedRaw); ok {
		updated = t
	}
	spaces := make([]string, 0, len(r.Spaces))
	for _, t := range r.Spaces {
		v := strings.TrimSpace(t.Value)
		if v != "" {
			spaces = append(spaces, v)
		}
	}
	tags := make([]string, 0, len(r.Tags))
	for _, t := range r.Tags {
		v := strings.TrimSpace(t.Value)
		if v != "" {
			tags = append(tags, v)
		}
	}
	typeVal := ""
	if r.Type != nil {
		typeVal = strings.TrimSpace(r.Type.Value)
	}
	return Entry{
		ID:          r.ID,
		Title:       strings.TrimSpace(r.Title),
		Slug:        slug,
		Body:        r.Body,
		Summary:     strings.TrimSpace(r.Summary),
		Type:        typeVal,
		PublishedAt: pub,
		SourceURL:   strings.TrimSpace(r.SourceURL),
		SourceTitle: strings.TrimSpace(r.SourceTitle),
		ImageURL:    strings.TrimSpace(r.ImageURL),
		Image:       append([]BaserowFile(nil), r.Image...),
		Markdown:    append([]BaserowFile(nil), r.Markdown...),
		Spaces:      spaces,
		Tags:        tags,
		CreatedAt:   created,
		UpdatedAt:   updated,
	}, true
}

func parseDate(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05.999999-07:00",
		"2006-01-02 15:04:05.999999Z07:00",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}
