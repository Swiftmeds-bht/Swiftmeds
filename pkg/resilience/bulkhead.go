package resilience

import "errors"

// ErrBulkheadFull is returned when the maximum number of calls are already running.
var ErrBulkheadFull = errors.New("resilience: bulkhead is full")

// Bulkhead limits how many calls run at the same time, so one slow
// service cannot use up every goroutine. It is safe for concurrent use.
type Bulkhead struct {
	slots chan struct{}
}

// NewBulkhead allows up to maxConcurrent calls at once. Values below 1 become 1.
func NewBulkhead(maxConcurrent int) *Bulkhead {
	return &Bulkhead{slots: make(chan struct{}, max(maxConcurrent, 1))}
}

// Execute runs fn if a slot is free. It does not wait: when every slot is
// taken it returns ErrBulkheadFull straight away.
func (b *Bulkhead) Execute(fn func() error) error {
	select {
	case b.slots <- struct{}{}:
		defer func() { <-b.slots }()
		return fn()
	default:
		return ErrBulkheadFull
	}
}
