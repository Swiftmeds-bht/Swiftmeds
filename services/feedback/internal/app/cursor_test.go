package app

import (
	"testing"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/services/feedback/internal/domain"
)

func TestCursorRoundTrip(t *testing.T) {
	in := domain.Cursor{CreatedAt: time.Date(2026, 11, 8, 9, 30, 15, 123456000, time.UTC), ID: "f-1|odd"}

	out, err := decodeCursor(encodeCursor(in))
	if err != nil {
		t.Fatal(err)
	}
	if out == nil || !out.CreatedAt.Equal(in.CreatedAt) || out.ID != in.ID {
		t.Errorf("cursor = %+v, want %+v", out, in)
	}
}

func TestDecodeCursorOfAnEmptyTokenIsTheFirstPage(t *testing.T) {
	c, err := decodeCursor("")
	if err != nil || c != nil {
		t.Errorf("decodeCursor(\"\") = %v, %v, want nil, nil", c, err)
	}
}
