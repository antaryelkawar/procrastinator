package tenant

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestWithTenant_TenantFrom_RoundTrip(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		id   string
	}{
		{"acme", "acme"},
		{"tenant-123", "tenant-123"},
		{"tenant_1", "tenant_1"},
		{"Acme-Corp", "Acme-Corp"},
		{"max-length-64", strings.Repeat("a", 64)},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := WithTenant(context.Background(), tc.id)
			got, err := TenantFrom(ctx)
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if got != tc.id {
				t.Fatalf("expected %q, got %q", tc.id, got)
			}
		})
	}
}

func TestTenantFrom_NoTenant(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	got, err := TenantFrom(ctx)
	if !errors.Is(err, ErrNoTenant) {
		t.Fatalf("expected ErrNoTenant, got %v", err)
	}
	if got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
}

func TestValid(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		id   string
		want bool
	}{
		{"valid simple", "acme", true},
		{"valid hyphen and digits", "tenant-123", true},
		{"valid underscore", "tenant_1", true},
		{"valid mixed case", "Acme-Corp", true},
		{"valid max length 64", strings.Repeat("a", 64), true},
		{"valid single char", "a", true},
		{"valid leading hyphen", "-leading", true},
		{"valid trailing hyphen", "trailing-", true},
		{"invalid empty", "", false},
		{"invalid too long 65", strings.Repeat("a", 65), false},
		{"invalid space", "has space", false},
		{"invalid slash", "a/b", false},
		{"invalid semicolon", "a;b", false},
		{"invalid non-ascii", "über", false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := Valid(tc.id); got != tc.want {
				t.Fatalf("Valid(%q) = %v, want %v", tc.id, got, tc.want)
			}
		})
	}
}

func TestWithTenant_InvalidIDRejected(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		id   string
	}{
		{"empty", ""},
		{"whitespace-only", "   "},
		{"has-space", "has space"},
		{"bad-char", "bad!char"},
		{"too-long", strings.Repeat("a", 65)},
		{"non-ascii", "über"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := WithTenant(context.Background(), tc.id)
			got, err := TenantFrom(ctx)
			if !errors.Is(err, ErrNoTenant) {
				t.Fatalf("expected ErrNoTenant for invalid id %q, got %v", tc.id, err)
			}
			if got != "" {
				t.Fatalf("expected empty string for invalid id %q, got %q", tc.id, got)
			}
		})
	}
}
