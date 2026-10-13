// Package config reads service settings from environment variables.
//
// A Loader collects every problem instead of stopping at the first one, so a
// misconfigured service reports all missing or invalid variables in one go:
//
//	cfg := config.New()
//	port := cfg.Int("PRICING_GRPC_PORT", 50063)
//	dbURL := cfg.Required("PRICING_DATABASE_URL")
//	if err := cfg.Err(); err != nil {
//		log.Fatal(err)
//	}
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"
)

// Loader reads environment variables and remembers any errors.
type Loader struct {
	lookup func(string) (string, bool)
	errs   []error
}

// New returns a Loader that reads the process environment.
func New() *Loader {
	return &Loader{lookup: os.LookupEnv}
}

// NewFromMap returns a Loader that reads from m instead of the environment. Used in tests.
func NewFromMap(m map[string]string) *Loader {
	return &Loader{lookup: func(k string) (string, bool) {
		v, ok := m[k]
		return v, ok
	}}
}

// Err returns all problems found so far, or nil.
func (l *Loader) Err() error {
	return errors.Join(l.errs...)
}

// get returns the value of key, treating an empty value as unset.
func (l *Loader) get(key string) (string, bool) {
	v, ok := l.lookup(key)
	if !ok || v == "" {
		return "", false
	}
	return v, true
}

// String returns the value of key, or def if it is unset.
func (l *Loader) String(key, def string) string {
	if v, ok := l.get(key); ok {
		return v
	}
	return def
}

// Required returns the value of key and records an error if it is unset.
func (l *Loader) Required(key string) string {
	v, ok := l.get(key)
	if !ok {
		l.errs = append(l.errs, fmt.Errorf("%s is required but not set", key))
	}
	return v
}

// Int returns key parsed as an int, or def if it is unset.
func (l *Loader) Int(key string, def int) int {
	v, ok := l.get(key)
	if !ok {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		l.errs = append(l.errs, fmt.Errorf("%s=%q is not an integer", key, v))
		return def
	}
	return n
}

// Bool returns key parsed as a bool (true/false/1/0), or def if it is unset.
func (l *Loader) Bool(key string, def bool) bool {
	v, ok := l.get(key)
	if !ok {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		l.errs = append(l.errs, fmt.Errorf("%s=%q is not a boolean", key, v))
		return def
	}
	return b
}

// Duration returns key parsed as a time.Duration (e.g. "15m"), or def if it is unset.
func (l *Loader) Duration(key string, def time.Duration) time.Duration {
	v, ok := l.get(key)
	if !ok {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		l.errs = append(l.errs, fmt.Errorf("%s=%q is not a duration", key, v))
		return def
	}
	return d
}
