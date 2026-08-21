package domain

import (
	"errors"
	"testing"
	"time"
)

type pageItem struct {
	ID string
	At time.Time
}

func TestPageRequestNormalize(t *testing.T) {
	normalized, err := (PageRequest{}).Normalize()
	if err != nil || normalized.Limit != DefaultPageSize {
		t.Fatalf("request=%+v err=%v", normalized, err)
	}
	if _, err := (PageRequest{Limit: MaximumPageSize + 1}).Normalize(); !errors.Is(err, ErrInvalid) {
		t.Fatalf("large limit error=%v", err)
	}
	if _, err := (PageRequest{Limit: -1}).Normalize(); !errors.Is(err, ErrInvalid) {
		t.Fatalf("negative limit error=%v", err)
	}
	if _, err := (PageRequest{Cursor: "not-base64"}).Normalize(); !errors.Is(err, ErrInvalid) {
		t.Fatalf("cursor error=%v", err)
	}
}

func TestCursorRoundTripPreservesUTCAndID(t *testing.T) {
	at := time.Date(2026, 8, 21, 9, 30, 0, 123, time.FixedZone("CST", 8*60*60))
	encoded, err := EncodeCursor(Cursor{SortTime: at, ID: "voyage-alpha"})
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeCursor(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.ID != "voyage-alpha" || !decoded.SortTime.Equal(at) || decoded.SortTime.Location() != time.UTC {
		t.Fatalf("cursor=%+v", decoded)
	}
}

func TestCursorRejectsMissingFields(t *testing.T) {
	if _, err := EncodeCursor(Cursor{ID: "voyage"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("missing time error=%v", err)
	}
	if _, err := EncodeCursor(Cursor{SortTime: time.Now()}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("missing id error=%v", err)
	}
	if _, err := DecodeCursor("e30"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("empty payload error=%v", err)
	}
}

func TestBuildPageReturnsStableNextCursor(t *testing.T) {
	base := time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC)
	items := []pageItem{
		{ID: "a", At: base},
		{ID: "b", At: base.Add(time.Hour)},
		{ID: "c", At: base.Add(2 * time.Hour)},
	}
	page, err := BuildPage(items, 2, func(item pageItem) Cursor {
		return Cursor{SortTime: item.At, ID: item.ID}
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 2 || !page.HasMore || page.NextCursor == "" {
		t.Fatalf("page=%+v", page)
	}
	cursor, err := DecodeCursor(page.NextCursor)
	if err != nil || cursor.ID != "b" || !cursor.SortTime.Equal(items[1].At) {
		t.Fatalf("cursor=%+v err=%v", cursor, err)
	}
	page.Items[0].ID = "changed"
	if items[0].ID != "a" {
		t.Fatal("page should own its item slice")
	}
}

func TestBuildPageWithoutExtraItemHasNoCursor(t *testing.T) {
	items := []pageItem{{ID: "a", At: time.Now()}}
	page, err := BuildPage(items, 2, func(item pageItem) Cursor { return Cursor{SortTime: item.At, ID: item.ID} })
	if err != nil {
		t.Fatal(err)
	}
	if page.HasMore || page.NextCursor != "" || len(page.Items) != 1 {
		t.Fatalf("page=%+v", page)
	}
}
