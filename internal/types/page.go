package types

// CursorPage is a cursor-paginated list request.
type CursorPage struct {
	Limit  int
	Cursor string
}

// CursorResult is Cloudflare result_info for cursor lists.
type CursorResult struct {
	Cursor  string `json:"cursor"`
	PerPage int    `json:"per_page"`
	Count   int    `json:"count"`
}

// OffsetPage is a page/per_page list request.
type OffsetPage struct {
	Page    int
	PerPage int
}

// OffsetResult is Cloudflare result_info for offset lists.
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

// NormalizeCursorPage applies CF list defaults and caps.
func NormalizeCursorPage(p CursorPage) CursorPage {
	if p.Limit <= 0 {
		p.Limit = defaultLimit
	}
	if p.Limit > maxLimit {
		p.Limit = maxLimit
	}
	return p
}

// NormalizeOffsetPage applies CF token-list defaults and caps.
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
