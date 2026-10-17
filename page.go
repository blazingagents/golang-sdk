package blazingagents

import (
	"context"
	"iter"
)

// Page is one page of a {data, nextCursor} list.
type Page[T any] struct {
	Data       []T    `json:"data"`
	NextCursor string `json:"nextCursor"` // "" on the last page; null decodes to ""
}

func (p *Page[T]) items() []T   { return p.Data }
func (p *Page[T]) next() string { return p.NextCursor }

// pager is implemented by Page and by every list page with another shape.
type pager[T any] interface {
	items() []T
	next() string
}

// all yields every item across pages. Nothing is sent until the first iteration. A fetch
// error is yielded once and ends the iteration; breaking out stops fetching. Ranging the
// sequence again starts over.
func all[T any, P pager[T]](ctx context.Context, fetch func(context.Context, string) (P, error)) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		var c string
		for {
			page, err := fetch(ctx, c)
			if err != nil {
				var zero T
				yield(zero, err)
				return
			}
			for _, it := range page.items() {
				if !yield(it, nil) {
					return
				}
			}
			if c = page.next(); c == "" {
				return
			}
		}
	}
}
