package domain

import "time"

// Cursor marks the last feedback of a page. Feedback is listed newest first, by
// created_at and then by ID, so the next page starts right after this one.
type Cursor struct {
	CreatedAt time.Time
	ID        string
}

// ListFilter selects feedback for FeedbackRepository.ListByPharmacy.
type ListFilter struct {
	PharmacyID string
	// After is where the page starts. Nil means the first page.
	After *Cursor
	// Limit is the most feedback to return. Zero or less means no limit.
	Limit int
}
