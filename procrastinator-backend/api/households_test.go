package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"procrastinator-backend/api/gen"
)

// TestCreateHousehold exercises POST /api/users/{userId}/households.
func TestCreateHousehold(t *testing.T) {
	t.Parallel()

	t.Run("HappyPath", func(t *testing.T) {
		e := newEnv(t, envOpts{})
		body := bytes.NewBufferString(`{"display_name":"My Family"}`)
		rec := do(t, e.handler, http.MethodPost, "/api/users/test-user/households", "", body, "application/json")

		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201 (body: %s)", rec.Code, rec.Body.String())
		}

		var hh gen.Household
		if err := json.Unmarshal(rec.Body.Bytes(), &hh); err != nil {
			t.Fatalf("unmarshal household: %v (body: %s)", err, rec.Body.String())
		}
		if hh.Id == "" {
			t.Error("id is empty, want non-empty")
		}
		if hh.DisplayName != "My Family" {
			t.Errorf("display_name = %q, want %q", hh.DisplayName, "My Family")
		}
		if hh.OwnerId != "test-user" {
			t.Errorf("owner_id = %q, want %q", hh.OwnerId, "test-user")
		}
		if hh.CreatedAt.IsZero() {
			t.Error("created_at is zero, want non-empty")
		}
		if len(hh.Members) != 1 {
			t.Fatalf("members count = %d, want 1", len(hh.Members))
		}
		if hh.Members[0].UserId != "test-user" {
			t.Errorf("members[0].user_id = %q, want %q", hh.Members[0].UserId, "test-user")
		}
		if hh.Members[0].CreatedAt.IsZero() {
			t.Error("members[0].created_at is zero, want non-empty")
		}
	})

	t.Run("BlankName", func(t *testing.T) {
		e := newEnv(t, envOpts{})
		body := bytes.NewBufferString(`{"display_name":""}`)
		rec := do(t, e.handler, http.MethodPost, "/api/users/test-user/households", "", body, "application/json")

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400 (body: %s)", rec.Code, rec.Body.String())
		}
		assertErrorEnvelope(t, rec)
	})

	t.Run("MissingName", func(t *testing.T) {
		e := newEnv(t, envOpts{})
		body := bytes.NewBufferString(`{}`)
		rec := do(t, e.handler, http.MethodPost, "/api/users/test-user/households", "", body, "application/json")

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400 (body: %s)", rec.Code, rec.Body.String())
		}
		assertErrorEnvelope(t, rec)
	})

	t.Run("MalformedJSON", func(t *testing.T) {
		e := newEnv(t, envOpts{})
		body := bytes.NewBufferString(`{"display_name":`)
		rec := do(t, e.handler, http.MethodPost, "/api/users/test-user/households", "", body, "application/json")

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400 (body: %s)", rec.Code, rec.Body.String())
		}
		assertErrorEnvelope(t, rec)
	})
}

// TestAddHouseholdMember exercises POST /api/users/{userId}/households/{householdId}/members.
func TestAddHouseholdMember(t *testing.T) {
	t.Parallel()

	t.Run("HappyPath", func(t *testing.T) {
		e := newEnv(t, envOpts{})

		// Create a household first.
		createBody := bytes.NewBufferString(`{"display_name":"Test Household"}`)
		createRec := do(t, e.handler, http.MethodPost, "/api/users/test-user/households", "", createBody, "application/json")
		if createRec.Code != http.StatusCreated {
			t.Fatalf("create status = %d, want 201 (body: %s)", createRec.Code, createRec.Body.String())
		}
		var created gen.Household
		if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
			t.Fatalf("unmarshal created: %v (body: %s)", err, createRec.Body.String())
		}

		// Add a member.
		addBody := bytes.NewBufferString(`{"user_id":"test-user-b"}`)
		rec := do(t, e.handler, http.MethodPost, "/api/users/test-user/households/"+created.Id+"/members", "", addBody, "application/json")
		if rec.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want 204 (body: %s)", rec.Code, rec.Body.String())
		}
		if got := rec.Body.String(); got != "" {
			t.Errorf("body = %q, want empty for 204", got)
		}
	})

	t.Run("Idempotent", func(t *testing.T) {
		e := newEnv(t, envOpts{})

		// Create a household first.
		createBody := bytes.NewBufferString(`{"display_name":"Test Household"}`)
		createRec := do(t, e.handler, http.MethodPost, "/api/users/test-user/households", "", createBody, "application/json")
		if createRec.Code != http.StatusCreated {
			t.Fatalf("create status = %d, want 201 (body: %s)", createRec.Code, createRec.Body.String())
		}
		var created gen.Household
		if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
			t.Fatalf("unmarshal created: %v (body: %s)", err, createRec.Body.String())
		}

		// Add a member twice (fresh buffers: the first call consumes its body).
		addBody := bytes.NewBufferString(`{"user_id":"test-user-b"}`)
		rec1 := do(t, e.handler, http.MethodPost, "/api/users/test-user/households/"+created.Id+"/members", "", addBody, "application/json")
		if rec1.Code != http.StatusNoContent {
			t.Fatalf("first add status = %d, want 204 (body: %s)", rec1.Code, rec1.Body.String())
		}

		addBody2 := bytes.NewBufferString(`{"user_id":"test-user-b"}`)
		rec2 := do(t, e.handler, http.MethodPost, "/api/users/test-user/households/"+created.Id+"/members", "", addBody2, "application/json")
		if rec2.Code != http.StatusNoContent {
			t.Fatalf("second add status = %d, want 204 (body: %s)", rec2.Code, rec2.Body.String())
		}
	})

	t.Run("RequesterNotMember", func(t *testing.T) {
		e := newEnv(t, envOpts{})

		// Create a household owned by test-user (test-user is member).
		createBody := bytes.NewBufferString(`{"display_name":"Test Household"}`)
		createRec := do(t, e.handler, http.MethodPost, "/api/users/test-user/households", "", createBody, "application/json")
		if createRec.Code != http.StatusCreated {
			t.Fatalf("create status = %d, want 201 (body: %s)", createRec.Code, createRec.Body.String())
		}
		var created gen.Household
		if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
			t.Fatalf("unmarshal created: %v (body: %s)", err, createRec.Body.String())
		}

		// test-user-b tries to add a member to a household they're not in.
		addBody := bytes.NewBufferString(`{"user_id":"test-user-b"}`)
		rec := do(t, e.handler, http.MethodPost, "/api/users/test-user-b/households/"+created.Id+"/members", "", addBody, "application/json")
		if rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403 (body: %s)", rec.Code, rec.Body.String())
		}
		assertErrorEnvelope(t, rec)
	})

	t.Run("UnknownHousehold", func(t *testing.T) {
		e := newEnv(t, envOpts{})

		addBody := bytes.NewBufferString(`{"user_id":"test-user-b"}`)
		rec := do(t, e.handler, http.MethodPost, "/api/users/test-user/households/unknown-household/members", "", addBody, "application/json")
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404 (body: %s)", rec.Code, rec.Body.String())
		}
		assertErrorEnvelope(t, rec)
	})

	t.Run("UnknownUser", func(t *testing.T) {
		e := newEnv(t, envOpts{})

		// Create a household first.
		createBody := bytes.NewBufferString(`{"display_name":"Test Household"}`)
		createRec := do(t, e.handler, http.MethodPost, "/api/users/test-user/households", "", createBody, "application/json")
		if createRec.Code != http.StatusCreated {
			t.Fatalf("create status = %d, want 201 (body: %s)", createRec.Code, createRec.Body.String())
		}
		var created gen.Household
		if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
			t.Fatalf("unmarshal created: %v (body: %s)", err, createRec.Body.String())
		}

		// Try to add a non-existent user.
		addBody := bytes.NewBufferString(`{"user_id":"nonexistent-user"}`)
		rec := do(t, e.handler, http.MethodPost, "/api/users/test-user/households/"+created.Id+"/members", "", addBody, "application/json")
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404 (body: %s)", rec.Code, rec.Body.String())
		}
		assertErrorEnvelope(t, rec)
	})

	t.Run("BlankUserID", func(t *testing.T) {
		e := newEnv(t, envOpts{})

		addBody := bytes.NewBufferString(`{"user_id":""}`)
		rec := do(t, e.handler, http.MethodPost, "/api/users/test-user/households/"+unknownUUID+"/members", "", addBody, "application/json")
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400 (body: %s)", rec.Code, rec.Body.String())
		}
		assertErrorEnvelope(t, rec)
	})
}

// TestListHouseholds exercises GET /api/users/{userId}/households.
func TestListHouseholds(t *testing.T) {
	t.Parallel()

	t.Run("Empty", func(t *testing.T) {
		e := newEnv(t, envOpts{})
		rec := do(t, e.handler, http.MethodGet, "/api/users/test-user/households", "", nil, "")

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}
		if got := strings.TrimSpace(rec.Body.String()); got != "[]" {
			t.Fatalf("body = %q, want exactly %q (never null)", got, "[]")
		}
	})

	t.Run("WithOwned", func(t *testing.T) {
		e := newEnv(t, envOpts{})

		// Create a household.
		createBody := bytes.NewBufferString(`{"display_name":"My Family"}`)
		createRec := do(t, e.handler, http.MethodPost, "/api/users/test-user/households", "", createBody, "application/json")
		if createRec.Code != http.StatusCreated {
			t.Fatalf("create status = %d, want 201 (body: %s)", createRec.Code, createRec.Body.String())
		}

		// List households.
		rec := do(t, e.handler, http.MethodGet, "/api/users/test-user/households", "", nil, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}

		var list []gen.Household
		if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
			t.Fatalf("unmarshal list: %v (body: %s)", err, rec.Body.String())
		}
		if len(list) != 1 {
			t.Fatalf("list count = %d, want 1", len(list))
		}
		if list[0].DisplayName != "My Family" {
			t.Errorf("display_name = %q, want %q", list[0].DisplayName, "My Family")
		}
		if list[0].OwnerId != "test-user" {
			t.Errorf("owner_id = %q, want %q", list[0].OwnerId, "test-user")
		}
		if len(list[0].Members) != 1 {
			t.Fatalf("members count = %d, want 1", len(list[0].Members))
		}
		if list[0].Members[0].UserId != "test-user" {
			t.Errorf("members[0].user_id = %q, want %q", list[0].Members[0].UserId, "test-user")
		}
	})

	t.Run("WithMembered", func(t *testing.T) {
		e := newEnv(t, envOpts{})

		// Create a household as test-user.
		createBody := bytes.NewBufferString(`{"display_name":"My Family"}`)
		createRec := do(t, e.handler, http.MethodPost, "/api/users/test-user/households", "", createBody, "application/json")
		if createRec.Code != http.StatusCreated {
			t.Fatalf("create status = %d, want 201 (body: %s)", createRec.Code, createRec.Body.String())
		}
		var created gen.Household
		if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
			t.Fatalf("unmarshal created: %v (body: %s)", err, createRec.Body.String())
		}

		// Add test-user-b as a member.
		addBody := bytes.NewBufferString(`{"user_id":"test-user-b"}`)
		addRec := do(t, e.handler, http.MethodPost, "/api/users/test-user/households/"+created.Id+"/members", "", addBody, "application/json")
		if addRec.Code != http.StatusNoContent {
			t.Fatalf("add member status = %d, want 204 (body: %s)", addRec.Code, addRec.Body.String())
		}

		// test-user-b should see the household in their list.
		rec := do(t, e.handler, http.MethodGet, "/api/users/test-user-b/households", "", nil, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}

		var list []gen.Household
		if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
			t.Fatalf("unmarshal list: %v (body: %s)", err, rec.Body.String())
		}
		if len(list) != 1 {
			t.Fatalf("list count = %d, want 1", len(list))
		}
		if list[0].Id != created.Id {
			t.Errorf("id = %q, want %q", list[0].Id, created.Id)
		}
		// Both members should be listed.
		if len(list[0].Members) != 2 {
			t.Fatalf("members count = %d, want 2", len(list[0].Members))
		}
		memberIDs := map[string]bool{}
		for _, m := range list[0].Members {
			memberIDs[m.UserId] = true
		}
		if !memberIDs["test-user"] {
			t.Error("test-user not in members")
		}
		if !memberIDs["test-user-b"] {
			t.Error("test-user-b not in members")
		}
	})
}

// TestGetHousehold exercises GET /api/users/{userId}/households/{householdId}.
func TestGetHousehold(t *testing.T) {
	t.Parallel()

	t.Run("Found", func(t *testing.T) {
		e := newEnv(t, envOpts{})

		// Create a household.
		createBody := bytes.NewBufferString(`{"display_name":"My Family"}`)
		createRec := do(t, e.handler, http.MethodPost, "/api/users/test-user/households", "", createBody, "application/json")
		if createRec.Code != http.StatusCreated {
			t.Fatalf("create status = %d, want 201 (body: %s)", createRec.Code, createRec.Body.String())
		}
		var created gen.Household
		if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
			t.Fatalf("unmarshal created: %v (body: %s)", err, createRec.Body.String())
		}

		// Get the household.
		rec := do(t, e.handler, http.MethodGet, "/api/users/test-user/households/"+created.Id, "", nil, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}

		var got gen.Household
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("unmarshal household: %v (body: %s)", err, rec.Body.String())
		}
		if got.Id != created.Id {
			t.Errorf("id = %q, want %q", got.Id, created.Id)
		}
		if got.DisplayName != "My Family" {
			t.Errorf("display_name = %q, want %q", got.DisplayName, "My Family")
		}
		if got.OwnerId != "test-user" {
			t.Errorf("owner_id = %q, want %q", got.OwnerId, "test-user")
		}
		if len(got.Members) != 1 {
			t.Fatalf("members count = %d, want 1", len(got.Members))
		}
		if got.Members[0].UserId != "test-user" {
			t.Errorf("members[0].user_id = %q, want %q", got.Members[0].UserId, "test-user")
		}
	})

	t.Run("NotFound", func(t *testing.T) {
		e := newEnv(t, envOpts{})
		rec := do(t, e.handler, http.MethodGet, "/api/users/test-user/households/unknown-household", "", nil, "")

		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404 (body: %s)", rec.Code, rec.Body.String())
		}
		assertErrorEnvelope(t, rec)
	})

	t.Run("NotVisible", func(t *testing.T) {
		e := newEnv(t, envOpts{})

		// Create a household owned by test-user (test-user is member).
		createBody := bytes.NewBufferString(`{"display_name":"Test Household"}`)
		createRec := do(t, e.handler, http.MethodPost, "/api/users/test-user/households", "", createBody, "application/json")
		if createRec.Code != http.StatusCreated {
			t.Fatalf("create status = %d, want 201 (body: %s)", createRec.Code, createRec.Body.String())
		}
		var created gen.Household
		if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
			t.Fatalf("unmarshal created: %v (body: %s)", err, createRec.Body.String())
		}

		// test-user-b tries to get a household they're not a member of.
		rec := do(t, e.handler, http.MethodGet, "/api/users/test-user-b/households/"+created.Id, "", nil, "")
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404 (body: %s)", rec.Code, rec.Body.String())
		}
		assertErrorEnvelope(t, rec)
	})
}
