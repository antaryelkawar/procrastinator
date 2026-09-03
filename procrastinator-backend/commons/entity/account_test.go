package entity

import (
	"testing"
	"time"
)

func TestFinancialAccountFieldRoundTrip(t *testing.T) {
	t.Parallel()

	p := func(v string) *string { return &v }
	createdAt := time.Date(2026, 8, 28, 10, 0, 0, 0, time.UTC)
	updatedAt := time.Date(2026, 8, 28, 11, 30, 0, 0, time.UTC)

	fa := FinancialAccount{
		ID:                 "acc-1",
		OwnerID:            "user-1",
		Name:               "Checking",
		Type:               AccountTypeBank,
		Currency:           "USD",
		Institution:        p("Acme Bank"),
		ExternalDescriptor: p("EXT-42"),
		CreatedAt:          createdAt,
		UpdatedAt:          updatedAt,
	}

	if fa.ID != "acc-1" {
		t.Fatalf("ID = %q, want %q", fa.ID, "acc-1")
	}
	if fa.OwnerID != "user-1" {
		t.Fatalf("OwnerID = %q, want %q", fa.OwnerID, "user-1")
	}
	if fa.Name != "Checking" {
		t.Fatalf("Name = %q, want %q", fa.Name, "Checking")
	}
	if fa.Type != "bank" {
		t.Fatalf("Type = %q, want %q", fa.Type, "bank")
	}
	if fa.Currency != "USD" {
		t.Fatalf("Currency = %q, want %q", fa.Currency, "USD")
	}
	if fa.Institution == nil {
		t.Fatal("Institution = nil, want non-nil pointer")
	}
	if *fa.Institution != "Acme Bank" {
		t.Fatalf("Institution = %q, want %q", *fa.Institution, "Acme Bank")
	}
	if fa.ExternalDescriptor == nil {
		t.Fatal("ExternalDescriptor = nil, want non-nil pointer")
	}
	if *fa.ExternalDescriptor != "EXT-42" {
		t.Fatalf("ExternalDescriptor = %q, want %q", *fa.ExternalDescriptor, "EXT-42")
	}
	if !fa.CreatedAt.Equal(createdAt) {
		t.Fatalf("CreatedAt = %q, want %q", fa.CreatedAt.Format(time.RFC3339), createdAt.Format(time.RFC3339))
	}
	if !fa.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("UpdatedAt = %q, want %q", fa.UpdatedAt.Format(time.RFC3339), updatedAt.Format(time.RFC3339))
	}
}

func TestValidAccountType(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"bank", "bank", true},
		{"wallet", "wallet", true},
		{"cash", "cash", true},
		{"credit_card", "credit_card", true},
		{"investment-unknown", "investment", false},
		{"empty", "", false},
		{"BANK-capitalized", "BANK", false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := ValidAccountType(tc.in)
			if got != tc.want {
				t.Fatalf("ValidAccountType(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestAccountTypeConstantValues(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		got  string
		want string
	}{
		{"AccountTypeBank", AccountTypeBank, "bank"},
		{"AccountTypeWallet", AccountTypeWallet, "wallet"},
		{"AccountTypeCash", AccountTypeCash, "cash"},
		{"AccountTypeCreditCard", AccountTypeCreditCard, "credit_card"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if tc.got != tc.want {
				t.Fatalf("constant %s = %q, want %q", tc.name, tc.got, tc.want)
			}
		})
	}
}

func TestAccountsMayShareName(t *testing.T) {
	t.Parallel()

	a := FinancialAccount{ID: "acc-1", OwnerID: "user-1", Name: "Checking", Type: AccountTypeBank, Currency: "USD"}
	b := FinancialAccount{ID: "acc-2", OwnerID: "user-1", Name: "Checking", Type: AccountTypeWallet, Currency: "USD"}

	if a.Name != b.Name {
		t.Fatalf("entity layer rejected shared name: a.Name = %q, b.Name = %q", a.Name, b.Name)
	}
	if a.Name != "Checking" {
		t.Fatalf("shared name = %q, want %q", a.Name, "Checking")
	}
}
