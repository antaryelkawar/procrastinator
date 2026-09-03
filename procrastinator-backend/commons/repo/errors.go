package repo

import "errors"

// ErrNotMember is returned by Create when the caller attempts to create a
// household-scoped row (owner_household_id set) but is not a member of that
// household. A later API task maps this sentinel to HTTP 403.
var ErrNotMember = errors.New("repo: caller is not a member of the target household")
