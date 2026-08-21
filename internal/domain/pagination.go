package domain

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const (
	DefaultPageSize = 50
	MaximumPageSize = 200
)

type PageRequest struct {
	Limit  int
	Cursor string
}

type Cursor struct {
	SortTime time.Time `json:"sort_time"`
	ID       string    `json:"id"`
}

type Page[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"next_cursor,omitempty"`
	HasMore    bool   `json:"has_more"`
}

func (request PageRequest) Normalize() (PageRequest, error) {
	if request.Limit < 0 || request.Limit > MaximumPageSize {
		return PageRequest{}, fmt.Errorf("%w: page limit", ErrInvalid)
	}
	if request.Limit == 0 {
		request.Limit = DefaultPageSize
	}
	request.Cursor = strings.TrimSpace(request.Cursor)
	if request.Cursor != "" {
		if _, err := DecodeCursor(request.Cursor); err != nil {
			return PageRequest{}, err
		}
	}
	return request, nil
}

func EncodeCursor(cursor Cursor) (string, error) {
	if cursor.SortTime.IsZero() || strings.TrimSpace(cursor.ID) == "" {
		return "", fmt.Errorf("%w: cursor", ErrInvalid)
	}
	cursor.SortTime = cursor.SortTime.UTC()
	payload, err := json.Marshal(cursor)
	if err != nil {
		return "", fmt.Errorf("encode cursor: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func DecodeCursor(encoded string) (Cursor, error) {
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		return Cursor{}, fmt.Errorf("%w: cursor encoding", ErrInvalid)
	}
	var cursor Cursor
	if err := json.Unmarshal(payload, &cursor); err != nil {
		return Cursor{}, fmt.Errorf("%w: cursor payload", ErrInvalid)
	}
	if cursor.SortTime.IsZero() || strings.TrimSpace(cursor.ID) == "" {
		return Cursor{}, fmt.Errorf("%w: cursor fields", ErrInvalid)
	}
	cursor.SortTime = cursor.SortTime.UTC()
	return cursor, nil
}

func BuildPage[T any](items []T, limit int, cursorFor func(T) Cursor) (Page[T], error) {
	if limit <= 0 || cursorFor == nil {
		return Page[T]{}, fmt.Errorf("%w: page builder", ErrInvalid)
	}
	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}
	page := Page[T]{Items: append([]T(nil), items...), HasMore: hasMore}
	if hasMore && len(items) > 0 {
		next, err := EncodeCursor(cursorFor(items[len(items)-1]))
		if err != nil {
			return Page[T]{}, err
		}
		page.NextCursor = next
	}
	return page, nil
}
