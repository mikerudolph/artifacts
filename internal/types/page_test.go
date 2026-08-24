package types

import "testing"

func TestNormalizeCursorPage(t *testing.T) {
	t.Parallel()
	got := NormalizeCursorPage(CursorPage{})
	if got.Limit != defaultLimit {
		t.Fatalf("default limit = %d", got.Limit)
	}
	got = NormalizeCursorPage(CursorPage{Limit: 999})
	if got.Limit != maxLimit {
		t.Fatalf("capped limit = %d", got.Limit)
	}
	got = NormalizeCursorPage(CursorPage{Limit: 10, Cursor: "abc"})
	if got.Limit != 10 || got.Cursor != "abc" {
		t.Fatalf("got %+v", got)
	}
}

func TestNormalizeOffsetPage(t *testing.T) {
	t.Parallel()
	got := NormalizeOffsetPage(OffsetPage{})
	if got.Page != 1 || got.PerPage != defaultPerPage {
		t.Fatalf("defaults %+v", got)
	}
	got = NormalizeOffsetPage(OffsetPage{Page: 2, PerPage: 999})
	if got.Page != 2 || got.PerPage != maxPerPage {
		t.Fatalf("capped %+v", got)
	}
}
