package entity

import (
	"testing"
	"time"
)

func TestHouseholdFields(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC().Truncate(time.Second)
	h := Household{
		ID:          "hh-1",
		OwnerID:     "user-1",
		DisplayName: "The Smiths",
		CreatedAt:   now,
	}

	if h.ID != "hh-1" {
		t.Fatalf("ID = %q, want %q", h.ID, "hh-1")
	}
	if h.OwnerID != "user-1" {
		t.Fatalf("OwnerID = %q, want %q", h.OwnerID, "user-1")
	}
	if h.DisplayName != "The Smiths" {
		t.Fatalf("DisplayName = %q, want %q", h.DisplayName, "The Smiths")
	}
	if !h.CreatedAt.Equal(now) {
		t.Fatalf("CreatedAt = %v, want %v", h.CreatedAt, now)
	}
}

func TestHouseholdMemberFields(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC().Truncate(time.Second)
	m := HouseholdMember{
		HouseholdID: "hh-1",
		UserID:      "user-1",
		CreatedAt:   now,
	}

	if m.HouseholdID != "hh-1" {
		t.Fatalf("HouseholdID = %q, want %q", m.HouseholdID, "hh-1")
	}
	if m.UserID != "user-1" {
		t.Fatalf("UserID = %q, want %q", m.UserID, "user-1")
	}
	if !m.CreatedAt.Equal(now) {
		t.Fatalf("CreatedAt = %v, want %v", m.CreatedAt, now)
	}
}
