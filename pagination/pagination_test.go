package pagination

import (
	"testing"
)

func TestPage_Params(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		p := Page{Offset: 0, Limit: 0}
		offset, limit := p.Params()
		if offset != 0 || limit != 20 {
			t.Fatalf("expected offset=0, limit=20, got %d, %d", offset, limit)
		}
	})

	t.Run("negative offset", func(t *testing.T) {
		p := Page{Offset: -10, Limit: 50}
		offset, limit := p.Params()
		if offset != 0 {
			t.Fatalf("expected offset=0, got %d", offset)
		}
		if limit != 50 {
			t.Fatalf("expected limit=50, got %d", limit)
		}
	})

	t.Run("limit too high", func(t *testing.T) {
		p := Page{Offset: 10, Limit: 500}
		offset, limit := p.Params()
		if limit != 100 {
			t.Fatalf("expected limit=100, got %d", limit)
		}
		if offset != 10 {
			t.Fatalf("expected offset=10, got %d", offset)
		}
	})
}

func TestCursor_Params(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		c := Cursor{After: "abc", Limit: 0}
		after, before, limit := c.Params()
		if after != "abc" || before != "" || limit != 20 {
			t.Fatalf("expected after=abc, before=, limit=20; got %q, %q, %d", after, before, limit)
		}
	})

	t.Run("limit capped at 100", func(t *testing.T) {
		c := Cursor{Limit: 999}
		_, _, limit := c.Params()
		if limit != 100 {
			t.Fatalf("expected limit=100, got %d", limit)
		}
	})
}

func TestBuildLinks_Cursor(t *testing.T) {
	links := BuildLinks("/api/users", "next123", "prev456", 50)

	if links.Self != "/api/users?limit=50" {
		t.Fatalf("unexpected self: %s", links.Self)
	}
	if links.First != "/api/users?limit=50" {
		t.Fatalf("unexpected first: %s", links.First)
	}
	if links.Next != "/api/users?after=next123&limit=50" {
		t.Fatalf("unexpected next: %s", links.Next)
	}
	if links.Prev != "/api/users?before=prev456&limit=50" {
		t.Fatalf("unexpected prev: %s", links.Prev)
	}
}

func TestBuildLinks_EmptyCursors(t *testing.T) {
	links := BuildLinks("/api/users", "", "", 20)

	if links.Next != "" {
		t.Fatalf("expected no next link")
	}
	if links.Prev != "" {
		t.Fatalf("expected no prev link")
	}
	if links.Self != "/api/users?limit=20" {
		t.Fatalf("unexpected self: %s", links.Self)
	}
}

func TestBuildPageLinks(t *testing.T) {
	// 100 total, offset 0, limit 20
	links := BuildPageLinks("/api/items", 0, 20, 100)

	if links.First != "/api/items?limit=20&offset=0" {
		t.Fatalf("unexpected first: %s", links.First)
	}
	if links.Next != "/api/items?limit=20&offset=20" {
		t.Fatalf("unexpected next: %s", links.Next)
	}
	if links.Prev != "" {
		t.Fatalf("expected no prev at start, got: %s", links.Prev)
	}
}

func TestBuildPageLinks_MiddlePage(t *testing.T) {
	// 100 total, offset 40, limit 20
	links := BuildPageLinks("/api/items", 40, 20, 100)

	if links.Prev != "/api/items?limit=20&offset=20" {
		t.Fatalf("unexpected prev: %s", links.Prev)
	}
	if links.Next != "/api/items?limit=20&offset=60" {
		t.Fatalf("unexpected next: %s", links.Next)
	}
}

func TestBuildPageLinks_LastPage(t *testing.T) {
	// 100 total, offset 80, limit 20
	links := BuildPageLinks("/api/items", 80, 20, 100)

	if links.Prev != "/api/items?limit=20&offset=60" {
		t.Fatalf("unexpected prev: %s", links.Prev)
	}
	if links.Next != "" {
		t.Fatalf("expected no next at end, got: %s", links.Next)
	}
}

func TestBuildPageLinks_ExistingQueryParams(t *testing.T) {
	links := BuildPageLinks("/api/items?filter=active", 0, 10, 50)

	if links.Next != "/api/items?filter=active&limit=10&offset=10" {
		t.Fatalf("unexpected next with query params: %s", links.Next)
	}
}

func TestBuildLinks_InvalidURL(t *testing.T) {
	links := BuildLinks("http://bad:host:80/", "", "", 20)
	// Should return empty links, not panic
	if links.Self != "" {
		t.Fatalf("expected empty links for invalid URL, got: %s", links.Self)
	}
}
