package manza

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
)

// MaxPerPage is the API's hard page-size cap.
const MaxPerPage = 100

// ListParams are the shared cursor-pagination inputs. Limit 0 means
// MaxPerPage.
type ListParams struct {
	Limit  int
	Cursor string
}

// Page is one page of a cursor-paginated list endpoint:
// { "data": [...], "has_more": bool, "next_cursor": string|null }.
type Page struct {
	Data       []map[string]any
	HasMore    bool
	NextCursor string
	Response   *Response

	fetch func(ctx context.Context, cursor string) (*Page, error)
}

// Next fetches the following page, or returns nil when this is the last one.
func (p *Page) Next(ctx context.Context) (*Page, error) {
	if !p.HasMore || p.NextCursor == "" {
		return nil, nil
	}
	return p.fetch(ctx, p.NextCursor)
}

func (c *Client) listPage(ctx context.Context, path string, base url.Values, params ListParams) (*Page, error) {
	limit := params.Limit
	if limit == 0 {
		limit = MaxPerPage
	}
	if limit < 0 || limit > MaxPerPage {
		return nil, fmt.Errorf("manza: limit must be between 1 and %d (got %d)", MaxPerPage, limit)
	}

	var fetch func(ctx context.Context, cursor string) (*Page, error)
	fetch = func(ctx context.Context, cursor string) (*Page, error) {
		query := url.Values{}
		for k, vs := range base {
			for _, v := range vs {
				query.Add(k, v)
			}
		}
		query.Set("limit", strconv.Itoa(limit))
		if cursor != "" {
			query.Set("cursor", cursor)
		}

		resp, err := c.get(ctx, path, query)
		if err != nil {
			return nil, err
		}

		page := &Page{Response: resp, fetch: fetch}
		if rows, ok := resp.Body["data"].([]any); ok {
			for _, row := range rows {
				if entry, ok := row.(map[string]any); ok {
					page.Data = append(page.Data, entry)
				}
			}
		}
		page.HasMore, _ = resp.Body["has_more"].(bool)
		page.NextCursor, _ = resp.Body["next_cursor"].(string)
		return page, nil
	}

	return fetch(ctx, params.Cursor)
}
