package web

import "testing"

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

func TestArchiveTypeCode(t *testing.T) {
	tests := []struct {
		id   int
		typ  string
		want string
	}{
		{7, "note", "FRG-0007"},
		{12, "Echo", "ECH-0012"},
		{9, "artifact", "ART-0009"},
		{2, "concept", "CON-0002"},
		{1, "unknown", "SB-0001"},
		{999, "Fragment", "FRG-0999"},
	}
	for _, tt := range tests {
		if got := archiveTypeCode(tt.id, tt.typ); got != tt.want {
			t.Fatalf("archiveTypeCode(%d, %q) = %q, want %q", tt.id, tt.typ, got, tt.want)
		}
	}
}
