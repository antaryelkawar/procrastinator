package user

import (
	"context"
	"errors"
	"regexp"
)

// ErrNoUser is returned when a context carries no user ID.
var ErrNoUser = errors.New("no user in context")

// userKey is the unexported key type for storing user ID in context.
type userKey struct{}

// userPattern validates user IDs: 1-64 characters, alphanumeric plus underscore and hyphen.
var userPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// Valid reports whether id is a syntactically valid user ID:
// 1-64 characters, alphanumeric plus underscore and hyphen.
func Valid(id string) bool {
	return userPattern.MatchString(id)
}

// WithUser returns a child context carrying the given user ID.
// If the ID is invalid (does not match the pattern), the original context is returned unchanged.
func WithUser(ctx context.Context, id string) context.Context {
	if !Valid(id) {
		return ctx
	}
	return context.WithValue(ctx, userKey{}, id)
}

// UserFrom extracts the user ID from the context.
// Returns ErrNoUser if no user is present or if the value is not a string.
func UserFrom(ctx context.Context) (string, error) {
	val := ctx.Value(userKey{})
	if val == nil {
		return "", ErrNoUser
	}
	id, ok := val.(string)
	if !ok {
		return "", ErrNoUser
	}
	return id, nil
}
