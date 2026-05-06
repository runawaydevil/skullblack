package baserow

import (
	"log"
	"net/url"
	"os"
	"strings"
)

type EntryImage struct {
	HasImage bool
	URL      string
	CardURL  string
	Alt      string
	Source   string
}

func ResolveEntryImage(entry Entry) EntryImage {
	// 1) Uploaded Baserow file
	for _, f := range entry.Image {
		if !isImageFile(f) {
			continue
		}

		fullURL := strings.TrimSpace(f.URL)
		if !isSafeImageURL(fullURL) {
			continue
		}

		cardURL := pickCardURL(f)
		if cardURL == "" {
			cardURL = fullURL
		}
		if !isSafeImageURL(cardURL) {
			cardURL = fullURL
		}

		alt := strings.TrimSpace(f.VisibleName)
		if alt == "" {
			alt = strings.TrimSpace(f.Name)
		}
		if alt == "" {
			alt = strings.TrimSpace(entry.Title)
		}

		out := EntryImage{
			HasImage: true,
			URL:      fullURL,
			CardURL:  cardURL,
			Alt:      alt,
			Source:   "file",
		}
		debugImage(entry, out, len(entry.Image))
		return out
	}

	// 2) External image URL
	raw := strings.TrimSpace(entry.ImageURL)
	if isSafeImageURL(raw) {
		out := EntryImage{
			HasImage: true,
			URL:      raw,
			CardURL:  raw,
			Alt:      strings.TrimSpace(entry.Title),
			Source:   "url",
		}
		debugImage(entry, out, len(entry.Image))
		return out
	}

	out := EntryImage{HasImage: false}
	debugImage(entry, out, len(entry.Image))
	return out
}

func isSafeImageURL(raw string) bool {
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
	case "javascript", "data", "file", "vbscript":
		return false
	default:
		// Relative URLs are not allowed for images in this app.
		return false
	}
}

func isImageFile(f BaserowFile) bool {
	if f.IsImage {
		return true
	}
	mt := strings.ToLower(strings.TrimSpace(f.MimeType))
	return strings.HasPrefix(mt, "image/")
}

func pickCardURL(f BaserowFile) string {
	if f.Thumbnails == nil {
		return ""
	}
	if t, ok := f.Thumbnails["card_cover"]; ok {
		return strings.TrimSpace(t.URL)
	}
	if t, ok := f.Thumbnails["small"]; ok {
		return strings.TrimSpace(t.URL)
	}
	return ""
}

func debugImage(entry Entry, img EntryImage, fileCount int) {
	if strings.ToLower(strings.TrimSpace(os.Getenv("DEBUG_IMAGES"))) != "true" {
		return
	}
	log.Printf(
		"images: title=%q image_url_present=%t image_file_count=%d resolved_source=%q resolved_card_url_present=%t resolved_url_present=%t",
		strings.TrimSpace(entry.Title),
		strings.TrimSpace(entry.ImageURL) != "",
		fileCount,
		img.Source,
		strings.TrimSpace(img.CardURL) != "",
		strings.TrimSpace(img.URL) != "",
	)
}
