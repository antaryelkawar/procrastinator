package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"procrastinator-backend/commons/entity"
	"procrastinator-backend/commons/repo"
)

// strPtr returns a pointer to s.
func strPtr(s string) *string { return &s }

// TestDeleteAsset exercises DELETE /api/users/{userId}/assets/{assetId}.
func TestDeleteAsset(t *testing.T) {
	t.Parallel()

	t.Run("HidesAsset", func(t *testing.T) {
		e := newEnv(t, envOpts{})
		up, asset := e.uploadFile(t, "test-user", "invoice.pdf", "application/pdf", pdfBytes(16))
		if up.Code != http.StatusCreated {
			t.Fatalf("upload status = %d, want 201 (body: %s)", up.Code, up.Body.String())
		}
		id := strVal(asset, "id")

		// Delete the asset.
		rec := do(t, e.handler, http.MethodDelete, "/api/users/test-user/assets/"+id, "", nil, "")
		if rec.Code != http.StatusNoContent {
			t.Fatalf("delete status = %d, want 204 (body: %s)", rec.Code, rec.Body.String())
		}

		// Subsequent GET without include_deleted returns 404.
		rec = do(t, e.handler, http.MethodGet, "/api/users/test-user/assets/"+id, "", nil, "")
		if rec.Code != http.StatusNotFound {
			t.Fatalf("get after delete status = %d, want 404 (body: %s)", rec.Code, rec.Body.String())
		}

		// GET with include_deleted=true returns 200 with deleted_at set.
		rec = do(t, e.handler, http.MethodGet, "/api/users/test-user/assets/"+id+"?include_deleted=true", "", nil, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("get with include_deleted status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}
		var got map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if got["deleted_at"] == nil {
			t.Errorf("deleted_at is nil, want non-nil (body: %s)", rec.Body.String())
		}
	})

	t.Run("UnknownAsset", func(t *testing.T) {
		e := newEnv(t, envOpts{})
		rec := do(t, e.handler, http.MethodDelete, "/api/users/test-user/assets/00000000-0000-4000-8000-000000000000", "", nil, "")
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404 (body: %s)", rec.Code, rec.Body.String())
		}
		assertErrorEnvelope(t, rec)
	})

	t.Run("AnotherOwner", func(t *testing.T) {
		e := newEnv(t, envOpts{})
		up, asset := e.uploadFile(t, "test-user", "invoice.pdf", "application/pdf", pdfBytes(16))
		if up.Code != http.StatusCreated {
			t.Fatalf("upload status = %d, want 201 (body: %s)", up.Code, up.Body.String())
		}
		id := strVal(asset, "id")

		// Another user cannot delete this asset.
		rec := do(t, e.handler, http.MethodDelete, "/api/users/test-user-b/assets/"+id, "", nil, "")
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404 (body: %s)", rec.Code, rec.Body.String())
		}
	})
}

// TestRestoreAsset exercises POST /api/users/{userId}/assets/{assetId}/restore.
func TestRestoreAsset(t *testing.T) {
	t.Parallel()

	t.Run("WithinWindow", func(t *testing.T) {
		e := newEnv(t, envOpts{})
		up, asset := e.uploadFile(t, "test-user", "invoice.pdf", "application/pdf", pdfBytes(16))
		if up.Code != http.StatusCreated {
			t.Fatalf("upload status = %d, want 201 (body: %s)", up.Code, up.Body.String())
		}
		id := strVal(asset, "id")

		// Delete then restore.
		rec := do(t, e.handler, http.MethodDelete, "/api/users/test-user/assets/"+id, "", nil, "")
		if rec.Code != http.StatusNoContent {
			t.Fatalf("delete status = %d, want 204 (body: %s)", rec.Code, rec.Body.String())
		}
		rec = do(t, e.handler, http.MethodPost, "/api/users/test-user/assets/"+id+"/restore", "", nil, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("restore status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}
		var got map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if got["deleted_at"] != nil {
			t.Errorf("deleted_at = %v, want nil after restore (body: %s)", got["deleted_at"], rec.Body.String())
		}

		// Asset is visible again without include_deleted.
		rec = do(t, e.handler, http.MethodGet, "/api/users/test-user/assets/"+id, "", nil, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("get after restore status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}
	})

	t.Run("UnknownAsset", func(t *testing.T) {
		e := newEnv(t, envOpts{})
		rec := do(t, e.handler, http.MethodPost, "/api/users/test-user/assets/00000000-0000-4000-8000-000000000000/restore", "", nil, "")
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404 (body: %s)", rec.Code, rec.Body.String())
		}
		assertErrorEnvelope(t, rec)
	})
}

// TestMergeAsset exercises POST /api/users/{userId}/assets/{assetId}/merge.
func TestMergeAsset(t *testing.T) {
	t.Parallel()

	t.Run("SurvivorHasBothDocs", func(t *testing.T) {
		e := newEnv(t, envOpts{})
		ctx := context.Background()

		// Create two assets directly (bypassing the upload pipeline).
		a1, err := e.factory.Assets.Create(ctx, entity.Asset{
			Brand: strPtr("Brand1"),
			Model: strPtr("M1"),
		}, repo.Owner("test-user"))
		if err != nil {
			t.Fatalf("create asset 1: %v", err)
		}
		a2, err := e.factory.Assets.Create(ctx, entity.Asset{
			Brand: strPtr("Brand2"),
			Model: strPtr("M2"),
		}, repo.Owner("test-user"))
		if err != nil {
			t.Fatalf("create asset 2: %v", err)
		}

		// Merge a2 into a1.
		body := `{"duplicate_asset_id":"` + a2.ID + `"}`
		req := newAuthedRequest(t, http.MethodPost, "/api/users/test-user/assets/"+a1.ID+"/merge", strings.NewReader(body), "application/json")
		rec := httptest.NewRecorder()
		e.handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("merge status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}
		var got map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		// Survivor ID is preserved.
		if got["id"] != a1.ID {
			t.Errorf("survivor id = %v, want %q", got["id"], a1.ID)
		}
		// merged_assets should contain a2.
		merged, ok := got["merged_assets"].([]any)
		if !ok || len(merged) == 0 {
			t.Fatalf("merged_assets missing or empty (body: %s)", rec.Body.String())
		}
	})

	t.Run("SelfMerge", func(t *testing.T) {
		e := newEnv(t, envOpts{})
		ctx := context.Background()
		a1, err := e.factory.Assets.Create(ctx, entity.Asset{}, repo.Owner("test-user"))
		if err != nil {
			t.Fatalf("create asset: %v", err)
		}
		body := `{"duplicate_asset_id":"` + a1.ID + `"}`
		req := newAuthedRequest(t, http.MethodPost, "/api/users/test-user/assets/"+a1.ID+"/merge", strings.NewReader(body), "application/json")
		rec := httptest.NewRecorder()
		e.handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400 (body: %s)", rec.Code, rec.Body.String())
		}
		assertErrorEnvelope(t, rec)
	})

	t.Run("UnknownAsset", func(t *testing.T) {
		e := newEnv(t, envOpts{})
		body := `{"duplicate_asset_id":"00000000-0000-4000-8000-000000000001"}`
		req := newAuthedRequest(t, http.MethodPost, "/api/users/test-user/assets/00000000-0000-4000-8000-000000000000/merge", strings.NewReader(body), "application/json")
		rec := httptest.NewRecorder()
		e.handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404 (body: %s)", rec.Code, rec.Body.String())
		}
		assertErrorEnvelope(t, rec)
	})
}

// TestPatchAsset exercises PATCH /api/users/{userId}/assets/{assetId}.
func TestPatchAsset(t *testing.T) {
	t.Parallel()

	t.Run("SetsCategory", func(t *testing.T) {
		e := newEnv(t, envOpts{})
		up, asset := e.uploadFile(t, "test-user", "invoice.pdf", "application/pdf", pdfBytes(16))
		if up.Code != http.StatusCreated {
			t.Fatalf("upload status = %d, want 201 (body: %s)", up.Code, up.Body.String())
		}
		id := strVal(asset, "id")

		body := `{"asset_category":"appliance"}`
		req := newAuthedRequest(t, http.MethodPatch, "/api/users/test-user/assets/"+id, strings.NewReader(body), "application/json")
		rec := httptest.NewRecorder()
		e.handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("patch status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}
		var got map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if got["asset_category"] != "appliance" {
			t.Errorf("asset_category = %v, want %q (body: %s)", got["asset_category"], "appliance", rec.Body.String())
		}
	})

	t.Run("SetsBrand", func(t *testing.T) {
		e := newEnv(t, envOpts{})
		up, asset := e.uploadFile(t, "test-user", "invoice.pdf", "application/pdf", pdfBytes(16))
		if up.Code != http.StatusCreated {
			t.Fatalf("upload status = %d, want 201 (body: %s)", up.Code, up.Body.String())
		}
		id := strVal(asset, "id")

		body := `{"brand":"NewBrand"}`
		req := newAuthedRequest(t, http.MethodPatch, "/api/users/test-user/assets/"+id, strings.NewReader(body), "application/json")
		rec := httptest.NewRecorder()
		e.handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("patch status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}
		var got map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if got["brand"] != "NewBrand" {
			t.Errorf("brand = %v, want %q (body: %s)", got["brand"], "NewBrand", rec.Body.String())
		}
	})

	t.Run("UnknownAsset", func(t *testing.T) {
		e := newEnv(t, envOpts{})
		body := `{"brand":"X"}`
		req := newAuthedRequest(t, http.MethodPatch, "/api/users/test-user/assets/00000000-0000-4000-8000-000000000000", strings.NewReader(body), "application/json")
		rec := httptest.NewRecorder()
		e.handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404 (body: %s)", rec.Code, rec.Body.String())
		}
		assertErrorEnvelope(t, rec)
	})
}
