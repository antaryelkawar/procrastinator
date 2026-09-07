package repo

// Option configures a repository query.
type Option func(*Options)

// Options holds query parameters for repository operations.
type Options struct {
	OwnerID string
	Filters  []Filter
	Limit    int
	Offset   int
	OrderBy  string

	// UpdateSets carries explicit column assignments applied on Update. A nil
	// value forces the column to SQL NULL. The generic Update otherwise only
	// SETs non-zero entity fields, so Set is how a caller clears a nullable
	// column. Field names are validated against the repository's column
	// whitelist on execution.
	UpdateSets map[string]any
}

// Filter represents a single WHERE clause condition.
type Filter struct {
	Field string
	Op    string // "=", "!=", ">", "<", ">=", "<=", "LIKE", "IN"
	Value any
}

// Owner sets the owner ID for owner-scoped queries.
func Owner(id string) Option {
	return func(o *Options) {
		o.OwnerID = id
	}
}

// Where adds a filter condition to the query.
func Where(field, op string, value any) Option {
	return func(o *Options) {
		o.Filters = append(o.Filters, Filter{Field: field, Op: op, Value: value})
	}
}

// Limit sets the maximum number of rows to return.
func Limit(n int) Option {
	return func(o *Options) {
		o.Limit = n
	}
}

// Offset sets the number of rows to skip.
func Offset(n int) Option {
	return func(o *Options) {
		o.Offset = n
	}
}

// OrderBy sets the ORDER BY clause.
func OrderBy(field string) Option {
	return func(o *Options) {
		o.OrderBy = field
	}
}

// Set records an explicit column assignment applied on Update. The value is
// stored verbatim in Options.UpdateSets; a nil value forces the column to SQL
// NULL (used to clear a nullable column such as assets.deleted_at or
// documents.asset_id). The field name is validated against the owning
// repository's column whitelist when the Update runs.
func Set(field string, value any) Option {
	return func(o *Options) {
		if o.UpdateSets == nil {
			o.UpdateSets = make(map[string]any)
		}
		o.UpdateSets[field] = value
	}
}

// ApplyOptions applies all options to a new Options struct and returns it.
func ApplyOptions(opts ...Option) *Options {
	o := &Options{}
	for _, fn := range opts {
		fn(o)
	}
	return o
}
