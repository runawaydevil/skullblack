package baserow

import "time"

// Entry is a published, public row ready for the site.
type Entry struct {
	ID          int
	Title       string
	Slug        string
	Body        string
	Summary     string
	Type        string
	PublishedAt time.Time
	SourceURL   string
	SourceTitle string
	ImageURL    string
	Image       []BaserowFile
	Markdown    []BaserowFile
	Spaces      []string
	Tags        []string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// DisplaySpaces returns Spaces when present, otherwise falls back to legacy Tags.
func (e Entry) DisplaySpaces() []string {
	if len(e.Spaces) > 0 {
		return e.Spaces
	}
	return e.Tags
}
