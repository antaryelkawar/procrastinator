package repo

// Option configures a repository query.
type Option func(*Options)

// Options holds query parameters for repository operations.
type Options struct {
	TenantID string
	Filters  []Filter
	Limit    int
	Offset   int
	OrderBy  string
}

// Filter represents a single WHERE clause condition.
type Filter struct {
	Field string
	Op    string // "=", "!=", ">", "<", ">=", "<=", "LIKE", "IN"
	Value any
}

// Tenant sets the tenant ID for tenant-scoped queries.
func Tenant(id string) Option {
	return func(o *Options) {
		o.TenantID = id
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

// ApplyOptions applies all options to a new Options struct and returns it.
func ApplyOptions(opts ...Option) *Options {
	o := &Options{}
	for _, fn := range opts {
		fn(o)
	}
	return o
}
