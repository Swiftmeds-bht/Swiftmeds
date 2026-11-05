package app

import (
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/services/holds/internal/domain"
)

func TestACursorSurvivesAnEncodeDecodeRoundTrip(t *testing.T) {
	want := domain.Cursor{CreatedAt: t0.Add(1234 * time.Microsecond), ID: "h-1|odd"}

	got, err := decodeCursor(encodeCursor(want))

	if err != nil {
		t.Fatal(err)
	}
	if got == nil || !got.CreatedAt.Equal(want.CreatedAt) || got.ID != want.ID {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestAnEmptyPageTokenMeansTheFirstPage(t *testing.T) {
	got, err := decodeCursor("")

	if err != nil || got != nil {
		t.Fatalf("got (%+v, %v), want (nil, nil)", got, err)
	}
}

func TestDecodeCursorRejectsTokensThisServiceDidNotMake(t *testing.T) {
	encode := func(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) }
	for _, token := range []string{"!!not base64!!", encode("no separator"), encode("abc|h-1"), encode("123|")} {
		if _, err := decodeCursor(token); !errors.Is(err, domain.ErrInvalidPageToken) {
			t.Errorf("token %q: err = %v, want ErrInvalidPageToken", token, err)
		}
	}
}
