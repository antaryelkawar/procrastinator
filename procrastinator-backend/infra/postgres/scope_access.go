package postgres

import (
	"context"
	"fmt"
	"strings"
)

// householdsForUser returns the ids of every household the given user is a
// member of. Returns a non-nil empty slice when the user belongs to no
// household.
func householdsForUser(ctx context.Context, q Querier, userID string) ([]string, error) {
	rows, err := q.Query(ctx, `SELECT household_id FROM household_members WHERE user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// visibilityCond builds the scope visibility predicate for a row per the D-8
// rule:
//
//	visible(row, me) := owner_id = me
//	                  OR (owner_household_id IS NOT NULL
//	                      AND owner_household_id IN households(me))
//
// where households(me) is the set of household ids the user is a member of.
//
// The predicate is composable: it appends its bind arguments to *args (the
// user id first, then each household id) and returns the SQL fragment with
// $-placeholders that continue the caller's existing arg numbering. No id is
// ever interpolated literally. It never emits an empty IN (): when the user
// has no households (or the repository is not shareable) the fragment is just
// "owner_id = $N".
//
// shareable gates whether the household disjunct is emitted at all. Tables
// without an owner_household_id column (e.g. households) pass shareable=false.
//
// table optionally qualifies both owner_id and owner_household_id with a
// table alias (e.g. "m" yields "m.owner_id = $N" / "m.owner_household_id IN
// (…)"). An empty table leaves the predicate unqualified (byte-identical to
// the no-alias form), which is what the main-table WHERE clauses use.
func visibilityCond(ctx context.Context, q Querier, userID string, shareable bool, table string, args *[]any) (string, error) {
	prefix := ""
	if table != "" {
		prefix = table + "."
	}

	if !shareable {
		*args = append(*args, userID)
		return fmt.Sprintf("%sowner_id = $%d", prefix, len(*args)), nil
	}

	households, err := householdsForUser(ctx, q, userID)
	if err != nil {
		return "", err
	}

	// User id is always the first bind arg of the fragment.
	*args = append(*args, userID)
	if len(households) == 0 {
		// No memberships: the household disjunct is impossible, so only the
		// user's personal rows are visible. No empty IN () is emitted.
		return fmt.Sprintf("%sowner_id = $%d", prefix, len(*args)), nil
	}

	placeholders := make([]string, len(households))
	for i, id := range households {
		*args = append(*args, id)
		placeholders[i] = fmt.Sprintf("$%d", len(*args))
	}
	return fmt.Sprintf("(%sowner_id = $%d OR %sowner_household_id IN (%s))", prefix, len(*args)-len(households), prefix, strings.Join(placeholders, ", ")), nil
}

// isMember reports whether userID is a member of householdID.
func isMember(ctx context.Context, q Querier, userID, householdID string) (bool, error) {
	var exists bool
	err := q.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM household_members WHERE user_id = $1 AND household_id = $2)`,
		userID, householdID,
	).Scan(&exists)
	return exists, err
}

// strFromAny reads a string (or *string) map value as a string. A nil or
// non-string value yields "".
func strFromAny(v any) string {
	switch s := v.(type) {
	case string:
		return s
	case *string:
		if s == nil {
			return ""
		}
		return *s
	default:
		return ""
	}
}
