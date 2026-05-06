package web

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"html/template"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"murad.world/murad-world/internal/baserow"
	"murad.world/murad-world/internal/config"
	"murad.world/murad-world/internal/content"
)

// Server handles HTTP requests.
type Server struct {
	cfg   config.Config
	store *content.Store
	tmpl  *template.Template
	intro string
}

// NewServer constructs the web server.
func NewServer(cfg config.Config, store *content.Store, tmpl *template.Template) *Server {
	return &Server{
		cfg:   cfg,
		store: store,
		tmpl:  tmpl,
		intro: "Ramblings and reveries.",
	}
}

// Register attaches routes to mux.
func (s *Server) Register(mux *http.ServeMux) {
	mux.Handle("GET /healthz", http.HandlerFunc(s.handleHealthz))
	mux.Handle("GET /robots.txt", http.HandlerFunc(s.handleRobots))
	mux.Handle("GET /sitemap.xml", http.HandlerFunc(s.handleSitemap))
	mux.Handle("GET /feed.xml", http.HandlerFunc(s.handleFeed))
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir(StaticDir))))

	mux.Handle("GET /{$}", http.HandlerFunc(s.handleIndex))
	mux.Handle("GET /entries/{slug}", http.HandlerFunc(s.handleEntry))
	mux.Handle("GET /notes", http.HandlerFunc(s.handleType("note", "Notes")))
	mux.Handle("GET /links", http.HandlerFunc(s.handleType("link", "Links")))
	mux.Handle("GET /quotes", http.HandlerFunc(s.handleType("quote", "Quotes")))
	mux.Handle("GET /concepts", http.HandlerFunc(s.handleType("concept", "Concepts")))
	mux.Handle("GET /spaces", http.HandlerFunc(s.handleSpacesIndex))
	mux.Handle("GET /spaces/{space}", http.HandlerFunc(s.handleSpace))
	mux.Handle("GET /tags", http.HandlerFunc(s.redirectTags))
	mux.Handle("GET /tags/{tag}", http.HandlerFunc(s.redirectTag))
}

func (s *Server) redirectTags(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/spaces", http.StatusMovedPermanently)
}

func (s *Server) redirectTag(w http.ResponseWriter, r *http.Request) {
	tag := strings.TrimSpace(r.PathValue("tag"))
	if tag == "" {
		http.Redirect(w, r, "/spaces", http.StatusMovedPermanently)
		return
	}
	http.Redirect(w, r, "/spaces/"+url.PathEscape(tag), http.StatusMovedPermanently)
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("ok"))
}

func (s *Server) handleRobots(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	siteURL := strings.TrimRight(strings.TrimSpace(s.cfg.SiteURL), "/")
	if siteURL == "" {
		_, _ = w.Write([]byte("User-agent: *\nAllow: /\n"))
		return
	}
	_, _ = w.Write([]byte("User-agent: *\nAllow: /\nSitemap: " + siteURL + "/sitemap.xml\n"))
}

type rss struct {
	XMLName xml.Name   `xml:"rss"`
	Version string     `xml:"version,attr"`
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Title         string    `xml:"title"`
	Link          string    `xml:"link"`
	Description   string    `xml:"description"`
	Language      string    `xml:"language,omitempty"`
	Generator     string    `xml:"generator,omitempty"`
	LastBuildDate string    `xml:"lastBuildDate,omitempty"`
	Items         []rssItem `xml:"item"`
}

type rssItem struct {
	Title       string     `xml:"title"`
	Link        string     `xml:"link"`
	GUID        rssGuid    `xml:"guid"`
	Description string     `xml:"description"`
	PubDate     string     `xml:"pubDate,omitempty"`
	Categories  []string   `xml:"category,omitempty"`
	Source      *rssSource `xml:"source,omitempty"`
}

type rssGuid struct {
	IsPermaLink bool   `xml:"isPermaLink,attr"`
	Value       string `xml:",chardata"`
}

type rssSource struct {
	URL   string `xml:"url,attr"`
	Title string `xml:",chardata"`
}

func (s *Server) handleFeed(w http.ResponseWriter, r *http.Request) {
	entries, err := s.store.All()
	if err != nil {
		http.Error(w, "Service temporarily unavailable.", http.StatusServiceUnavailable)
		return
	}

	sortEntriesNewestFirst(entries)

	const maxItems = 50
	if len(entries) > maxItems {
		entries = entries[:maxItems]
	}

	lastBuild := newestBuildTime(entries)

	items := make([]rssItem, 0, len(entries))
	for _, e := range entries {
		base := e.Entry
		link := strings.TrimRight(s.cfg.SiteURL, "/") + "/entries/" + url.PathEscape(base.Slug)
		desc := strings.TrimSpace(base.Summary)
		if desc == "" {
			desc = excerptPlain(e.PlainText, 300)
		}

		var pubDate string
		if !base.PublishedAt.IsZero() {
			pubDate = base.PublishedAt.UTC().Format(time.RFC1123Z)
		}

		var src *rssSource
		if safeURL, ok := parseHTTPURL(base.SourceURL); ok {
			title := strings.TrimSpace(base.SourceTitle)
			if title == "" {
				title = sourceDomainFromURL(safeURL)
			}
			if title == "" {
				title = safeURL
			}
			src = &rssSource{URL: safeURL, Title: title}
		}

		items = append(items, rssItem{
			Title:       strings.TrimSpace(base.Title),
			Link:        link,
			GUID:        rssGuid{IsPermaLink: true, Value: link},
			Description: desc,
			PubDate:     pubDate,
			Categories:  append([]string(nil), base.DisplaySpaces()...),
			Source:      src,
		})
	}

	feed := rss{
		Version: "2.0",
		Channel: rssChannel{
			Title:         s.cfg.SiteName,
			Link:          s.cfg.SiteURL,
			Description:   "Ramblings and reveries.",
			Language:      "en",
			Generator:     "skull.black",
			LastBuildDate: lastBuild,
			Items:         items,
		},
	}

	w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(xml.Header))
	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	if err := enc.Encode(feed); err != nil {
		http.Error(w, "Failed to render feed.", http.StatusInternalServerError)
		return
	}
}

func newestBuildTime(entries []content.PreparedEntry) string {
	var newest time.Time
	for _, e := range entries {
		base := e.Entry
		if !base.UpdatedAt.IsZero() && base.UpdatedAt.After(newest) {
			newest = base.UpdatedAt
		}
		if !base.PublishedAt.IsZero() && base.PublishedAt.After(newest) {
			newest = base.PublishedAt
		}
	}
	if newest.IsZero() {
		return ""
	}
	return newest.UTC().Format(time.RFC1123Z)
}

func excerptPlain(s string, max int) string {
	s = strings.TrimSpace(s)
	if s == "" || max <= 0 {
		return ""
	}
	// Normalize whitespace to keep RSS description readable.
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= max {
		return s
	}
	// Keep it simple; avoid rune-splitting complexity for now (boring implementation).
	return strings.TrimSpace(s[:max]) + "…"
}

type latestView struct {
	Title       string
	Slug        string
	SlugEscaped string
	Type        string
	Date        string
}

func effectiveDate(e content.PreparedEntry) (time.Time, bool) {
	base := e.Entry
	if !base.PublishedAt.IsZero() {
		return base.PublishedAt, true
	}
	if !base.CreatedAt.IsZero() {
		return base.CreatedAt, true
	}
	return time.Time{}, false
}

func displayDate(e content.PreparedEntry) string {
	base := e.Entry
	if !base.PublishedAt.IsZero() {
		return formatDate(base.PublishedAt)
	}
	if !base.CreatedAt.IsZero() {
		return formatDate(base.CreatedAt)
	}
	return ""
}

func sortEntriesNewestFirst(entries []content.PreparedEntry) {
	sort.SliceStable(entries, func(i, j int) bool {
		ti, okI := effectiveDate(entries[i])
		tj, okJ := effectiveDate(entries[j])
		if okI && okJ {
			if ti.Equal(tj) {
				if entries[i].Entry.ID != entries[j].Entry.ID {
					return entries[i].Entry.ID > entries[j].Entry.ID
				}
				return entries[i].Entry.Slug < entries[j].Entry.Slug
			}
			return ti.After(tj)
		}
		if okI != okJ {
			return okI // dated first
		}
		// both undated: deterministic fallback
		if entries[i].Entry.ID != entries[j].Entry.ID {
			return entries[i].Entry.ID > entries[j].Entry.ID
		}
		return entries[i].Entry.Slug < entries[j].Entry.Slug
	})
}

func latest(entries []content.PreparedEntry, n int) []latestView {
	if n <= 0 || len(entries) == 0 {
		return nil
	}
	if len(entries) < n {
		n = len(entries)
	}
	out := make([]latestView, 0, n)
	for i := 0; i < n; i++ {
		e := entries[i].Entry
		out = append(out, latestView{
			Title:       strings.TrimSpace(e.Title),
			Slug:        e.Slug,
			SlugEscaped: url.PathEscape(e.Slug),
			Type:        strings.TrimSpace(e.Type),
			Date:        displayDate(entries[i]),
		})
	}
	return out
}

type urlset struct {
	XMLName xml.Name
	Xmlns   string   `xml:"xmlns,attr"`
	URLs    []urlLoc `xml:"url"`
}

type urlLoc struct {
	Loc     string `xml:"loc"`
	LastMod string `xml:"lastmod,omitempty"`
}

func (s *Server) handleSitemap(w http.ResponseWriter, r *http.Request) {
	entries, err := s.store.All()
	if err != nil {
		http.Error(w, "Service temporarily unavailable.", http.StatusServiceUnavailable)
		return
	}

	sortEntriesNewestFirst(entries)

	us := urlset{
		XMLName: xml.Name{Local: "urlset"},
		Xmlns:   "http://www.sitemaps.org/schemas/sitemap/0.9",
		URLs: []urlLoc{
			{Loc: s.cfg.SiteURL + "/"},
			{Loc: s.cfg.SiteURL + "/notes"},
			{Loc: s.cfg.SiteURL + "/links"},
			{Loc: s.cfg.SiteURL + "/quotes"},
			{Loc: s.cfg.SiteURL + "/concepts"},
			{Loc: s.cfg.SiteURL + "/spaces"},
		},
	}
	for _, e := range entries {
		base := e.Entry
		loc := s.cfg.SiteURL + "/entries/" + url.PathEscape(base.Slug)
		item := urlLoc{Loc: loc}
		if !base.UpdatedAt.IsZero() {
			item.LastMod = base.UpdatedAt.UTC().Format(time.RFC3339)
		} else if !base.PublishedAt.IsZero() {
			item.LastMod = base.PublishedAt.UTC().Format(time.RFC3339)
		}
		us.URLs = append(us.URLs, item)
	}

	for _, t := range allSpaces(entries) {
		us.URLs = append(us.URLs, urlLoc{
			Loc: s.cfg.SiteURL + "/spaces/" + url.PathEscape(t),
		})
	}

	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	_, _ = w.Write([]byte(xml.Header))
	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	if err := enc.Encode(us); err != nil {
		http.Error(w, "Failed to render sitemap.", http.StatusInternalServerError)
	}
}

type indexView struct {
	SiteName      string
	PageTitle     string
	Subtitle      string
	SiteURL       string
	Intro         string
	EntryCount    int
	Entries       []entryView
	EntriesJSON   template.HTML
	Counts        typeCounts
	LatestEntries []latestView
}

type typeCounts struct {
	Notes    int
	Links    int
	Quotes   int
	Concepts int
}

type entryView struct {
	Title           string
	Slug            string
	SlugEscaped     string
	Image           baserow.EntryImage
	Summary         string
	Type            string
	Published       string
	Tags            []string
	RenderedHTML    template.HTML
	Body            string
	HasMarkdownFile bool
	SourceURL       string
	SourceTitle     string
	SourceDomain    string
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	entries, err := s.store.All()
	if err != nil {
		s.renderUnavailable(w, http.StatusServiceUnavailable)
		return
	}

	sortEntriesNewestFirst(entries)

	data := indexView{
		SiteName:      s.cfg.SiteName,
		PageTitle:     "Index",
		Subtitle:      "",
		SiteURL:       s.cfg.SiteURL,
		Intro:         "",
		EntryCount:    len(entries),
		Entries:       toEntryViews(entries),
		EntriesJSON:   toEntriesJSON(entries),
		Counts:        countTypes(entries),
		LatestEntries: latest(entries, 5),
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if err := s.tmpl.ExecuteTemplate(w, "index.html", data); err != nil {
		http.Error(w, "Failed to render page.", http.StatusInternalServerError)
	}
}

func (s *Server) handleType(typeValue string, pageTitle string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		entries, err := s.store.All()
		if err != nil {
			s.renderUnavailable(w, http.StatusServiceUnavailable)
			return
		}

		sortEntriesNewestFirst(entries)
		filtered := filterByType(entries, typeValue)
		data := indexView{
			SiteName:      s.cfg.SiteName,
			PageTitle:     pageTitle,
			Subtitle:      "Newest first.",
			SiteURL:       s.cfg.SiteURL,
			Intro:         "",
			EntryCount:    len(filtered),
			Entries:       toEntryViews(filtered),
			EntriesJSON:   toEntriesJSON(filtered),
			Counts:        countTypes(entries),
			LatestEntries: latest(entries, 5),
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		if err := s.tmpl.ExecuteTemplate(w, "index.html", data); err != nil {
			http.Error(w, "Failed to render page.", http.StatusInternalServerError)
		}
	}
}

type tagsIndexView struct {
	SiteName      string
	PageTitle     string
	SiteURL       string
	Intro         string
	Counts        typeCounts
	LatestEntries []latestView
	Tags          []tagCountView
}

type tagCountView struct {
	Tag   string
	Count int
	Href  string
}

func (s *Server) handleSpacesIndex(w http.ResponseWriter, r *http.Request) {
	entries, err := s.store.All()
	if err != nil {
		s.renderUnavailable(w, http.StatusServiceUnavailable)
		return
	}

	sortEntriesNewestFirst(entries)
	spaces := spaceCounts(entries)
	out := make([]tagCountView, 0, len(spaces))
	for k, v := range spaces {
		out = append(out, tagCountView{Tag: k, Count: v, Href: url.PathEscape(k)})
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Tag) < strings.ToLower(out[j].Tag) })

	data := tagsIndexView{
		SiteName:      s.cfg.SiteName,
		PageTitle:     "Spaces",
		SiteURL:       s.cfg.SiteURL,
		Intro:         "",
		Counts:        countTypes(entries),
		LatestEntries: latest(entries, 5),
		Tags:          out,
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if err := s.tmpl.ExecuteTemplate(w, "tags.html", data); err != nil {
		http.Error(w, "Failed to render page.", http.StatusInternalServerError)
	}
}

func (s *Server) handleSpace(w http.ResponseWriter, r *http.Request) {
	space := strings.TrimSpace(r.PathValue("space"))
	if space == "" {
		http.NotFound(w, r)
		return
	}

	entries, err := s.store.All()
	if err != nil {
		s.renderUnavailable(w, http.StatusServiceUnavailable)
		return
	}

	sortEntriesNewestFirst(entries)
	filtered := filterBySpace(entries, space)
	data := indexView{
		SiteName:      s.cfg.SiteName,
		PageTitle:     "Space: " + space,
		Subtitle:      "Newest first.",
		SiteURL:       s.cfg.SiteURL,
		Intro:         "",
		EntryCount:    len(filtered),
		Entries:       toEntryViews(filtered),
		EntriesJSON:   toEntriesJSON(filtered),
		Counts:        countTypes(entries),
		LatestEntries: latest(entries, 5),
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if err := s.tmpl.ExecuteTemplate(w, "index.html", data); err != nil {
		http.Error(w, "Failed to render page.", http.StatusInternalServerError)
	}
}

type entryPageView struct {
	SiteName      string
	PageTitle     string
	SiteURL       string
	Entry         entryView
	HasSummary    bool
	HasSource     bool
	Counts        typeCounts
	LatestEntries []latestView
}

func (s *Server) handleEntry(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimSpace(r.PathValue("slug"))
	if slug == "" {
		http.NotFound(w, r)
		return
	}

	e, err := s.store.BySlug(slug)
	if err != nil {
		s.renderUnavailable(w, http.StatusServiceUnavailable)
		return
	}
	if e == nil {
		http.NotFound(w, r)
		return
	}

	base := e.Entry
	safeSourceURL, okSrc := parseHTTPURL(base.SourceURL)
	img := baserow.ResolveEntryImage(base)
	ev := entryView{
		Title:           base.Title,
		Slug:            base.Slug,
		SlugEscaped:     url.PathEscape(base.Slug),
		Image:           img,
		Summary:         base.Summary,
		Type:            base.Type,
		Published:       formatDate(base.PublishedAt),
		Tags:            append([]string(nil), base.DisplaySpaces()...),
		RenderedHTML:    e.RenderedHTML,
		Body:            base.Body,
		HasMarkdownFile: e.HasMarkdownFile,
		SourceURL:       safeSourceURL,
		SourceTitle:     base.SourceTitle,
	}

	title := base.Title
	if title == "" {
		title = base.Slug
	}

	data := entryPageView{
		SiteName:      s.cfg.SiteName,
		PageTitle:     title,
		SiteURL:       s.cfg.SiteURL,
		Entry:         ev,
		HasSummary:    strings.TrimSpace(base.Summary) != "",
		HasSource:     okSrc,
		Counts:        s.sidebarCounts(),
		LatestEntries: latestEntriesSafe(s, 5),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if err := s.tmpl.ExecuteTemplate(w, "entry.html", data); err != nil {
		http.Error(w, "Failed to render page.", http.StatusInternalServerError)
	}
}

func (s *Server) renderUnavailable(w http.ResponseWriter, code int) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(code)
	_ = s.tmpl.ExecuteTemplate(w, "unavailable.html", struct {
		SiteName      string
		PageTitle     string
		Counts        typeCounts
		LatestEntries []latestView
	}{
		SiteName:      s.cfg.SiteName,
		PageTitle:     "Unavailable",
		Counts:        s.sidebarCounts(),
		LatestEntries: latestEntriesSafe(s, 5),
	})
}

func formatDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format("2006-01-02")
}

func sourceDomainFromURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	host := strings.TrimSpace(u.Hostname())
	if host == "" {
		return ""
	}
	host = strings.TrimPrefix(host, "www.")
	return host
}

func parseHTTPURL(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	u, err := url.Parse(raw)
	if err != nil || u == nil {
		return "", false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", false
	}
	if strings.TrimSpace(u.Host) == "" {
		return "", false
	}
	return u.String(), true
}

func (s *Server) sidebarCounts() typeCounts {
	entries, err := s.store.All()
	if err != nil {
		return typeCounts{}
	}
	return countTypes(entries)
}

func latestEntriesSafe(s *Server, n int) []latestView {
	entries, err := s.store.All()
	if err != nil {
		return nil
	}
	sortEntriesNewestFirst(entries)
	return latest(entries, n)
}

func countTypes(entries []content.PreparedEntry) typeCounts {
	var counts typeCounts
	for _, e := range entries {
		switch strings.ToLower(strings.TrimSpace(e.Entry.Type)) {
		case "note", "notes":
			counts.Notes++
		case "link", "links":
			counts.Links++
		case "quote", "quotes":
			counts.Quotes++
		case "concept", "concepts":
			counts.Concepts++
		}
	}
	return counts
}

type entryIndexJSON struct {
	Slug         string   `json:"slug"`
	SlugEscaped  string   `json:"slug_escaped,omitempty"`
	Title        string   `json:"title"`
	Type         string   `json:"type"`
	Published    string   `json:"published"`
	Summary      string   `json:"summary"`
	Body         string   `json:"body"`
	SourceURL    string   `json:"source_url"`
	SourceTitle  string   `json:"source_title"`
	SourceDomain string   `json:"source_domain"`
	ImageURL     string   `json:"image_url,omitempty"`
	ImageCardURL string   `json:"image_card_url,omitempty"`
	ImageAlt     string   `json:"image_alt,omitempty"`
	Spaces       []string `json:"spaces,omitempty"`
	Tags         []string `json:"tags,omitempty"`
}

func toEntryViews(entries []content.PreparedEntry) []entryView {
	out := make([]entryView, 0, len(entries))
	for _, e := range entries {
		base := e.Entry
		safeSourceURL, _ := parseHTTPURL(base.SourceURL)
		img := baserow.ResolveEntryImage(base)
		out = append(out, entryView{
			Title:        base.Title,
			Slug:         base.Slug,
			SlugEscaped:  url.PathEscape(base.Slug),
			Image:        img,
			Summary:      base.Summary,
			Type:         base.Type,
			Published:    formatDate(base.PublishedAt),
			Tags:         append([]string(nil), base.DisplaySpaces()...),
			SourceURL:    safeSourceURL,
			SourceTitle:  strings.TrimSpace(base.SourceTitle),
			SourceDomain: sourceDomainFromURL(safeSourceURL),
		})
	}
	return out
}

func toEntriesJSON(entries []content.PreparedEntry) template.HTML {
	jsonEntries := make([]entryIndexJSON, 0, len(entries))
	for _, e := range entries {
		base := e.Entry
		safeSourceURL, _ := parseHTTPURL(base.SourceURL)
		sourceDomain := sourceDomainFromURL(safeSourceURL)
		img := baserow.ResolveEntryImage(base)
		jsonEntries = append(jsonEntries, entryIndexJSON{
			Slug:         base.Slug,
			SlugEscaped:  url.PathEscape(base.Slug),
			Title:        base.Title,
			Type:         base.Type,
			Published:    formatDate(base.PublishedAt),
			Summary:      base.Summary,
			Body:         e.PreviewText,
			SourceURL:    safeSourceURL,
			SourceTitle:  base.SourceTitle,
			SourceDomain: sourceDomain,
			ImageURL:     img.URL,
			ImageCardURL: img.CardURL,
			ImageAlt:     img.Alt,
			Spaces:       append([]string(nil), base.DisplaySpaces()...),
			Tags:         append([]string(nil), base.Tags...),
		})
	}

	b, err := json.Marshal(jsonEntries)
	if err != nil {
		return ""
	}
	var buf bytes.Buffer
	json.HTMLEscape(&buf, b)
	return template.HTML(buf.String())
}

func filterByType(entries []content.PreparedEntry, typeValue string) []content.PreparedEntry {
	want := strings.ToLower(strings.TrimSpace(typeValue))
	out := make([]content.PreparedEntry, 0, len(entries))
	for _, e := range entries {
		got := strings.ToLower(strings.TrimSpace(e.Entry.Type))
		if got == want || got == want+"s" {
			out = append(out, e)
		}
	}
	return out
}

func filterBySpace(entries []content.PreparedEntry, space string) []content.PreparedEntry {
	want := strings.ToLower(strings.TrimSpace(space))
	out := make([]content.PreparedEntry, 0, len(entries))
	for _, e := range entries {
		for _, t := range e.Entry.DisplaySpaces() {
			if strings.ToLower(strings.TrimSpace(t)) == want {
				out = append(out, e)
				break
			}
		}
	}
	return out
}

func spaceCounts(entries []content.PreparedEntry) map[string]int {
	m := make(map[string]int)
	for _, e := range entries {
		for _, t := range e.Entry.DisplaySpaces() {
			v := strings.TrimSpace(t)
			if v == "" {
				continue
			}
			m[v]++
		}
	}
	return m
}

func allSpaces(entries []content.PreparedEntry) []string {
	m := spaceCounts(entries)
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i]) < strings.ToLower(out[j]) })
	return out
}
