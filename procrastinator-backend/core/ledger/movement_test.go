package ledger

import (
	"errors"
	"testing"

	"procrastinator-backend/commons/entity"
)

func TestValidateKindShape(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		kind     string
		sourceID string
		destID   string
		wantErr  bool
	}{
		{"expense-source-only", entity.KindExpense, "acc-1", "", false},
		{"expense-no-source", entity.KindExpense, "", "", true},
		{"expense-both", entity.KindExpense, "acc-1", "acc-2", true},
		{"income-destination-only", entity.KindIncome, "", "acc-2", false},
		{"income-source-only", entity.KindIncome, "acc-1", "", true},
		{"income-both", entity.KindIncome, "acc-1", "acc-2", true},
		{"income-neither", entity.KindIncome, "", "", true},
		{"transfer-distinct", entity.KindTransfer, "acc-1", "acc-2", false},
		{"transfer-same-both-sides", entity.KindTransfer, "acc-1", "acc-1", true},
		{"transfer-missing-source", entity.KindTransfer, "", "acc-2", true},
		{"transfer-missing-destination", entity.KindTransfer, "acc-1", "", true},
		{"unknown-kind", "debit", "acc-1", "acc-2", true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := validateKindShape(tc.kind, tc.sourceID, tc.destID)
			if (err != nil) != tc.wantErr {
				t.Fatalf("validateKindShape(%q, %q, %q) = %v, wantErr %v", tc.kind, tc.sourceID, tc.destID, err, tc.wantErr)
			}
			if tc.wantErr && !errors.Is(err, ErrInvalid) {
				t.Fatalf("validateKindShape(%q, %q, %q) = %v, want ErrInvalid", tc.kind, tc.sourceID, tc.destID, err)
			}
		})
	}
}

func TestCurrencyMatches(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		kind     string
		accounts map[string]entity.FinancialAccount
		currency string
		wantErr  bool
	}{
		{
			"all-match",
			entity.KindTransfer,
			map[string]entity.FinancialAccount{
				"acc-1": {ID: "acc-1", Currency: "INR"},
				"acc-2": {ID: "acc-2", Currency: "INR"},
			},
			"INR",
			false,
		},
		{
			"one-mismatched",
			entity.KindTransfer,
			map[string]entity.FinancialAccount{
				"acc-1": {ID: "acc-1", Currency: "INR"},
				"acc-2": {ID: "acc-2", Currency: "USD"},
			},
			"INR",
			true,
		},
		{
			"both-mismatched",
			entity.KindTransfer,
			map[string]entity.FinancialAccount{
				"acc-1": {ID: "acc-1", Currency: "USD"},
				"acc-2": {ID: "acc-2", Currency: "EUR"},
			},
			"INR",
			true,
		},
		{
			"unknown-kind",
			"debit",
			map[string]entity.FinancialAccount{
				"acc-1": {ID: "acc-1", Currency: "INR"},
			},
			"INR",
			true,
		},
		{
			"empty-map-valid-kind",
			entity.KindExpense,
			map[string]entity.FinancialAccount{},
			"INR",
			false,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := currencyMatches(tc.kind, tc.accounts, tc.currency)
			if (err != nil) != tc.wantErr {
				t.Fatalf("currencyMatches(%q, %d accounts, %q) = %v, wantErr %v", tc.kind, len(tc.accounts), tc.currency, err, tc.wantErr)
			}
			if tc.wantErr && !errors.Is(err, ErrInvalid) {
				t.Fatalf("currencyMatches(%q, %d accounts, %q) = %v, want ErrInvalid", tc.kind, len(tc.accounts), tc.currency, err)
			}
		})
	}
}
