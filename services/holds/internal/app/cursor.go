package app

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/services/holds/internal/domain"
)

// A page token is the last hold of the page, as "<created_at in microseconds>|<hold id>",
// base64-encoded so callers treat it as opaque.

func encodeCursor(c domain.Cursor) string {
	raw := fmt.Sprintf("%d|%s", c.CreatedAt.UnixMicro(), c.ID)
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// decodeCursor turns a page token back into a cursor. An empty token means the
// first page and gives a nil cursor.
func decodeCursor(token string) (*domain.Cursor, error) {
	if token == "" {
		return nil, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return nil, domain.ErrInvalidPageToken
	}
	micros, id, ok := strings.Cut(string(raw), "|")
	if !ok || id == "" {
		return nil, domain.ErrInvalidPageToken
	}
	n, err := strconv.ParseInt(micros, 10, 64)
	if err != nil {
		return nil, domain.ErrInvalidPageToken
	}
	return &domain.Cursor{CreatedAt: time.UnixMicro(n).UTC(), ID: id}, nil
}
