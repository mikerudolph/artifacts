package types

type CursorPage struct {
	Limit  int
	Cursor string
}

type CursorResult struct {
	Cursor  string `json:"cursor"`
	PerPage int    `json:"per_page"`
	Count   int    `json:"count"`
}

type OffsetPage struct {
	Page    int
	PerPage int
}

type OffsetResult struct {
	Page       int `json:"page"`
	PerPage    int `json:"per_page"`
	TotalPages int `json:"total_pages"`
	Count      int `json:"count"`
	TotalCount int `json:"total_count"`
}

const (
	defaultLimit   = 50
	maxLimit       = 200
	defaultPerPage = 30
	maxPerPage     = 100
)

func NormalizeCursorPage(p CursorPage) CursorPage {
	if p.Limit <= 0 {
		p.Limit = defaultLimit
	}
	if p.Limit > maxLimit {
		p.Limit = maxLimit
	}
	return p
}

func NormalizeOffsetPage(p OffsetPage) OffsetPage {
	if p.Page <= 0 {
		p.Page = 1
	}
	if p.PerPage <= 0 {
		p.PerPage = defaultPerPage
	}
	if p.PerPage > maxPerPage {
		p.PerPage = maxPerPage
	}
	return p
}
