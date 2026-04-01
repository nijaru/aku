package pagination

import (
	"fmt"
	"net/url"
)

// Page represents offset-based pagination.
type Page struct {
	Offset int `query:"offset" doc:"Number of records to skip"`
	Limit  int `query:"limit" doc:"Maximum number of records to return"`
}

// Cursor represents cursor-based pagination.
type Cursor struct {
	After  string `query:"after" doc:"Cursor pointing to the start of the next page"`
	Before string `query:"before" doc:"Cursor pointing to the start of the previous page"`
	Limit  int    `query:"limit" doc:"Maximum number of records to return"`
}

// PageParams wraps Page and Limit with clamping/defaults.
func (p *Page) Params() (offset, limit int) {
	offset = p.Offset
	if offset < 0 {
		offset = 0
	}
	limit = p.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	return offset, limit
}

// CursorParams wraps Cursor and Limit with clamping/defaults.
func (c *Cursor) Params() (after, before string, limit int) {
	after = c.After
	before = c.Before
	limit = c.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	return after, before, limit
}

// Links holds navigation URLs for a paginated response.
type Links struct {
	Self  string `json:"self"`
	Next  string `json:"next,omitempty"`
	Prev  string `json:"prev,omitempty"`
	First string `json:"first"`
}

// PageResponse is a paginated response for offset-based pagination.
type PageResponse[T any] struct {
	Data  []T   `json:"data"`
	Total int64 `json:"total"`
	Links Links `json:"links"`
}

// CursorResponse is a paginated response for cursor-based pagination.
type CursorResponse[T any] struct {
	Data  []T    `json:"data"`
	Links Links  `json:"links"`
	HasMore bool  `json:"has_more"`
}

// BuildLinks creates pagination links from the current request URL
// and cursor values. Callers pass in the current request URL and the
// cursors for the next/previous pages. If there is no next/previous page,
// omit the values or pass empty strings.
func BuildLinks(rawURL, nextCursor, prevCursor string, limit int) Links {
	u, err := url.Parse(rawURL)
	if err != nil {
		return Links{}
	}

	q := u.Query()
	q.Set("limit", fmt.Sprintf("%d", limit))

	first := *u
	first.RawQuery = q.Encode()

	self := *u
	self.RawQuery = q.Encode()

	result := Links{
		Self:  self.String(),
		First: first.String(),
	}

	if nextCursor != "" {
		next := *u
		nq := next.Query()
		nq.Set("after", nextCursor)
		nq.Set("limit", fmt.Sprintf("%d", limit))
		next.RawQuery = nq.Encode()
		result.Next = next.String()
	}

	if prevCursor != "" {
		prev := *u
		pq := prev.Query()
		pq.Set("before", prevCursor)
		pq.Set("limit", fmt.Sprintf("%d", limit))
		prev.RawQuery = pq.Encode()
		result.Prev = prev.String()
	}

	return result
}

// BuildPageLinks creates pagination links for offset-based pagination.
func BuildPageLinks(rawURL string, offset, limit int, total int64) Links {
	hasMore := int64(offset+limit) < total
	hasPrev := offset > 0

	links := Links{
		First: buildPageURL(rawURL, 0, limit),
		Self:  buildPageURL(rawURL, offset, limit),
	}

	if hasMore {
		links.Next = buildPageURL(rawURL, offset+limit, limit)
	}
	if hasPrev {
		prevOffset := offset - limit
		if prevOffset < 0 {
			prevOffset = 0
		}
		links.Prev = buildPageURL(rawURL, prevOffset, limit)
	}

	return links
}

func buildPageURL(rawURL string, offset, limit int) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	q := u.Query()
	q.Set("offset", fmt.Sprintf("%d", offset))
	q.Set("limit", fmt.Sprintf("%d", limit))
	u.RawQuery = q.Encode()
	return u.String()
}
