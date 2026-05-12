package web

import (
	"testing"
	"time"

	"murad.world/murad-world/internal/baserow"
	"murad.world/murad-world/internal/content"
)

func TestNormalizeTypeBucket(t *testing.T) {
	tests := []struct {
		raw  string
		want string
	}{
		{"", ""},
		{"   ", ""},
		{"unknown", ""},
		{"Note", "note"},
		{"notes", "note"},
		{"Fragment", "note"},
		{"FRAGMENTS", "note"},
		{"link", "link"},
		{"Echo", "link"},
		{"echoes", "link"},
		{"quote", "quote"},
		{"Artifact", "quote"},
		{"artifacts", "quote"},
		{"concept", "concept"},
		{"Concepts", "concept"},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			if got := normalizeTypeBucket(tt.raw); got != tt.want {
				t.Fatalf("normalizeTypeBucket(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestFormatArchiveCode(t *testing.T) {
	tests := []struct {
		typ  string
		ord  int
		want string
	}{
		{"note", 1, "FRG-0001"},
		{"note", 10, "FRG-0010"},
		{"Echo", 12, "ECH-0012"},
		{"artifact", 9, "ART-0009"},
		{"concept", 2, "CON-0002"},
		{"unknown", 1, "SB-0001"},
		{"Fragment", 999, "FRG-0999"},
	}
	for _, tt := range tests {
		if got := formatArchiveCode(tt.typ, tt.ord); got != tt.want {
			t.Fatalf("formatArchiveCode(%q, %d) = %q, want %q", tt.typ, tt.ord, got, tt.want)
		}
	}
}

func TestBuildArchiveOrdinalByID_noteChronologicalIgnoresRowID(t *testing.T) {
	dOld := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	dNew := time.Date(2021, 6, 1, 0, 0, 0, 0, time.UTC)
	entries := []content.PreparedEntry{
		{Entry: baserow.Entry{ID: 10, Type: "note", PublishedAt: dNew, Slug: "b"}},
		{Entry: baserow.Entry{ID: 5, Type: "note", PublishedAt: dOld, Slug: "a"}},
	}
	ord := buildArchiveOrdinalByID(entries)
	if ord[5] != 1 || ord[10] != 2 {
		t.Fatalf("ordinals = %#v, want id5=1 id10=2", ord)
	}
	if got := formatArchiveCode("note", ord[10]); got != "FRG-0002" {
		t.Fatalf("newer note = %q, want FRG-0002", got)
	}
	if got := formatArchiveCode("note", ord[5]); got != "FRG-0001" {
		t.Fatalf("older note = %q, want FRG-0001", got)
	}
}

func TestBuildArchiveOrdinalByID_samePublishedAtLowerIDFirst(t *testing.T) {
	ts := time.Date(2022, 3, 3, 12, 0, 0, 0, time.UTC)
	entries := []content.PreparedEntry{
		{Entry: baserow.Entry{ID: 20, Type: "link", PublishedAt: ts, Slug: "z"}},
		{Entry: baserow.Entry{ID: 7, Type: "link", PublishedAt: ts, Slug: "y"}},
	}
	ord := buildArchiveOrdinalByID(entries)
	if ord[7] != 1 || ord[20] != 2 {
		t.Fatalf("ordinals = %#v, want id7=1 id20=2", ord)
	}
}

func TestBuildArchiveOrdinalByID_uncategorizedUsesSBSequence(t *testing.T) {
	d1 := time.Date(2019, 1, 1, 0, 0, 0, 0, time.UTC)
	d2 := time.Date(2019, 6, 1, 0, 0, 0, 0, time.UTC)
	entries := []content.PreparedEntry{
		{Entry: baserow.Entry{ID: 100, Type: "weird", PublishedAt: d2}},
		{Entry: baserow.Entry{ID: 50, Type: "odd", PublishedAt: d1}},
	}
	ord := buildArchiveOrdinalByID(entries)
	if ord[50] != 1 || ord[100] != 2 {
		t.Fatalf("ordinals = %#v, want id50=1 id100=2", ord)
	}
	if got := archiveDisplayCode(entries[0].Entry, ord); got != "SB-0002" {
		t.Fatalf("archiveDisplayCode = %q, want SB-0002", got)
	}
}

func TestArchiveDisplayCode_nilMapFallsBackToID(t *testing.T) {
	e := baserow.Entry{ID: 42, Type: "note"}
	if got := archiveDisplayCode(e, nil); got != "SB-0042" {
		t.Fatalf("got %q, want SB-0042", got)
	}
}
