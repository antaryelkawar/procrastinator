package postgres

import (
	"testing"

	"procrastinator-backend/commons/repo"
)

// TestCodecOwnsFilters verifies (design D3) that each entity kind's codec
// declares its own filter/order whitelist: every codec must carry a non-nil
// filterConfig with non-empty fieldCols and orderCols. This pins the codec as
// the single source of truth for the dynamic-query whitelists, so the generic
// engine reads r.codec.filters rather than a separately passed config.
func TestCodecOwnsFilters(t *testing.T) {
	t.Parallel()

	codecs := []struct {
		name string
		c    any // *payloadCodec[T] of some kind
	}{
		{"asset", assetCodec},
		{"source", sourceCodec},
		{"document", documentCodec},
		{"account", accountCodec},
		{"movement", movementCodec},
		{"import_batch", importBatchCodec},
		{"import_line", importLineCodec},
		{"review", reviewCodec},
		{"household", householdCodec},
	}

	for _, tc := range codecs {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := tc.c.(interface{ filterConfig() filterConfig })
			f := c.filterConfig()
			if len(f.fieldCols) == 0 {
				t.Fatalf("codec %s: fieldCols is empty, want non-empty", tc.name)
			}
			if len(f.orderCols) == 0 {
				t.Fatalf("codec %s: orderCols is empty, want non-empty", tc.name)
			}
		})
	}
}

// TestCodecFiltersReachEngine verifies that a repository built purely from a
// codec (no separately passed filter config) still validates and applies
// dynamic queries through the codec's filter config: a whitelisted field
// passes validation and a non-whitelisted field is rejected.
func TestCodecFiltersReachEngine(t *testing.T) {
	t.Parallel()

	// assetRepo builds a repository from assetCodec with no explicit filters.
	r := assetRepo(&recordingQuerier{})

	// Whitelisted field + valid operator: validateFilters accepts it.
	if err := validateFilters(r.codec.filters, []repo.Filter{{Field: "brand", Op: "="}}); err != nil {
		t.Fatalf("validateFilters(brand =) = %v, want nil", err)
	}
	// Non-whitelisted field: rejected.
	if err := validateFilters(r.codec.filters, []repo.Filter{{Field: "nope", Op: "="}}); err == nil {
		t.Fatal("validateFilters(nope =) = nil, want unknown filter field error")
	}
}
