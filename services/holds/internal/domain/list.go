package domain

import "time"

// Cursor marks the last hold of a page. Holds are listed newest first, by
// created_at and then by ID, so the next page starts right after this hold.
type Cursor struct {
	CreatedAt time.Time
	ID        string
}

// ListFilter selects holds for HoldRepository.List.
type ListFilter struct {
	// UserID or PharmacyID (or both) narrow the list. Empty means any.
	UserID     string
	PharmacyID string

	// Status narrows the list to one status. Empty means all statuses.
	Status Status

	// After is where the page starts. Nil means the first page.
	After *Cursor

	// Limit is the most holds to return. Zero or less means no limit.
	Limit int
}
