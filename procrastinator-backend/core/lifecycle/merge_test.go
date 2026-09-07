package lifecycle

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"procrastinator-backend/commons/entity"
	"procrastinator-backend/commons/repo"
	"procrastinator-backend/commons/user"
)

// ---------------------------------------------------------------------------
// TestMerge
// ---------------------------------------------------------------------------

func TestMerge(t *testing.T) {
	t.Parallel()

	t.Run("duplicate serial fills a null survivor serial", func(t *testing.T) {
		t.Parallel()
		seedAssets := []entity.Asset{
			{ID: "asset-1", OwnerID: testUser, Model: ptr("M1"), SerialNumber: nil},
			{ID: "asset-2", OwnerID: testUser, Model: ptr("M2"), SerialNumber: ptr("SN123")},
		}
		h := newHarness(t, 30, func() time.Time { return testNow }, seedAssets, nil, nil)

		if _, err := h.svc.Merge(testCtx(), "asset-1", "asset-2"); err != nil {
			t.Fatalf("Merge returned error %v, want nil", err)
		}
		a, ok := h.ff.assetRepo.get("asset-1")
		if !ok {
			t.Fatalf("asset-1 not found after merge")
		}
		if a.SerialNumber == nil || *a.SerialNumber != "SN123" {
			t.Fatalf("survivor SerialNumber = %v, want %q", a.SerialNumber, "SN123")
		}
	})

	t.Run("survivor model wins on conflict", func(t *testing.T) {
		t.Parallel()
		seedAssets := []entity.Asset{
			{ID: "asset-1", OwnerID: testUser, Model: ptr("CD600")},
			{ID: "asset-2", OwnerID: testUser, Model: ptr("CD600-BLK")},
		}
		h := newHarness(t, 30, func() time.Time { return testNow }, seedAssets, nil, nil)

		if _, err := h.svc.Merge(testCtx(), "asset-1", "asset-2"); err != nil {
			t.Fatalf("Merge returned error %v, want nil", err)
		}
		a, ok := h.ff.assetRepo.get("asset-1")
		if !ok {
			t.Fatalf("asset-1 not found after merge")
		}
		if a.Model == nil || *a.Model != "CD600" {
			t.Fatalf("survivor Model = %v, want %q (survivor wins on conflict)", a.Model, "CD600")
		}
	})

	t.Run("survivor user-set category is not overwritten", func(t *testing.T) {
		t.Parallel()
		seedAssets := []entity.Asset{
			{ID: "asset-1", OwnerID: testUser, CategoryUserSet: true, AssetCategory: ptr("furniture")},
			{ID: "asset-2", OwnerID: testUser, AssetCategory: ptr("computing")},
		}
		h := newHarness(t, 30, func() time.Time { return testNow }, seedAssets, nil, nil)

		if _, err := h.svc.Merge(testCtx(), "asset-1", "asset-2"); err != nil {
			t.Fatalf("Merge returned error %v, want nil", err)
		}
		a, ok := h.ff.assetRepo.get("asset-1")
		if !ok {
			t.Fatalf("asset-1 not found after merge")
		}
		if a.AssetCategory == nil || *a.AssetCategory != "furniture" {
			t.Fatalf("survivor AssetCategory = %v, want %q (user-set category is sticky)", a.AssetCategory, "furniture")
		}
		if !a.CategoryUserSet {
			t.Fatalf("survivor CategoryUserSet = false, want true (preserved)")
		}
	})

	t.Run("user-set survivor category (nil) is not filled from duplicate", func(t *testing.T) {
		t.Parallel()
		seedAssets := []entity.Asset{
			{ID: "asset-1", OwnerID: testUser, CategoryUserSet: true, AssetCategory: nil},
			{ID: "asset-2", OwnerID: testUser, AssetCategory: ptr("computing")},
		}
		h := newHarness(t, 30, func() time.Time { return testNow }, seedAssets, nil, nil)

		if _, err := h.svc.Merge(testCtx(), "asset-1", "asset-2"); err != nil {
			t.Fatalf("Merge returned error %v, want nil", err)
		}
		a, ok := h.ff.assetRepo.get("asset-1")
		if !ok {
			t.Fatalf("asset-1 not found after merge")
		}
		if a.AssetCategory != nil {
			t.Fatalf("survivor AssetCategory = %v, want nil (user-set category is sticky, not filled from the duplicate)", *a.AssetCategory)
		}
		if !a.CategoryUserSet {
			t.Fatalf("survivor CategoryUserSet = false, want true (preserved)")
		}
	})

	t.Run("metadata shallow-merged, survivor wins", func(t *testing.T) {
		t.Parallel()
		seedAssets := []entity.Asset{
			{ID: "asset-1", OwnerID: testUser, Metadata: map[string]any{"a": 1, "b": 2}},
			{ID: "asset-2", OwnerID: testUser, Metadata: map[string]any{"b": 99, "c": 3}},
		}
		h := newHarness(t, 30, func() time.Time { return testNow }, seedAssets, nil, nil)

		if _, err := h.svc.Merge(testCtx(), "asset-1", "asset-2"); err != nil {
			t.Fatalf("Merge returned error %v, want nil", err)
		}
		a, ok := h.ff.assetRepo.get("asset-1")
		if !ok {
			t.Fatalf("asset-1 not found after merge")
		}
		want := map[string]any{"a": 1, "b": 2, "c": 3}
		if !reflect.DeepEqual(a.Metadata, want) {
			t.Fatalf("survivor Metadata = %v, want %v (shallow merge, survivor wins)", a.Metadata, want)
		}
	})

	t.Run("survivor with D1 + duplicate with D2 keeps both docs", func(t *testing.T) {
		t.Parallel()
		seedAssets := []entity.Asset{
			{ID: "asset-1", OwnerID: testUser},
			{ID: "asset-2", OwnerID: testUser},
		}
		seedDocs := []entity.Document{
			{ID: "doc-1", OwnerID: testUser, SourceID: "s1", AssetID: "asset-1", DocType: "invoice"},
			{ID: "doc-2", OwnerID: testUser, SourceID: "s2", AssetID: "asset-2", DocType: "warranty"},
		}
		h := newHarness(t, 30, func() time.Time { return testNow }, seedAssets, seedDocs, nil)

		if _, err := h.svc.Merge(testCtx(), "asset-1", "asset-2"); err != nil {
			t.Fatalf("Merge returned error %v, want nil", err)
		}
		docs, err := h.ff.docRepo.List(testCtx(), repo.Owner(testUser), repo.Where("asset_id", "=", "asset-1"))
		if err != nil {
			t.Fatalf("doc List returned error %v, want nil", err)
		}
		got := make(map[string]bool, len(docs))
		for _, d := range docs {
			if d.AssetID != "asset-1" {
				t.Fatalf("doc %s AssetID = %q, want %q (must be on the survivor)", d.ID, d.AssetID, "asset-1")
			}
			got[d.ID] = true
		}
		if len(docs) != 2 || !got["doc-1"] || !got["doc-2"] {
			ids := make([]string, 0, len(docs))
			for _, d := range docs {
				ids = append(ids, d.ID)
			}
			t.Fatalf("docs linked to survivor = %v, want both doc-1 and doc-2", ids)
		}
		if h.ff.docRepo.count() != 2 {
			t.Fatalf("total doc count = %d, want 2 (no doc erased)", h.ff.docRepo.count())
		}
		d2, ok := h.ff.docRepo.get("doc-2")
		if !ok {
			t.Fatalf("doc-2 not found after merge")
		}
		if d2.AssetID != "asset-1" {
			t.Fatalf("doc-2 AssetID = %q, want %q (re-pointed from asset-2 to the survivor)", d2.AssetID, "asset-1")
		}
	})

	t.Run("duplicate gets merged_into + merged_at and is excluded from the active list", func(t *testing.T) {
		t.Parallel()
		seedAssets := []entity.Asset{
			{ID: "asset-1", OwnerID: testUser, Model: ptr("M1")},
			{ID: "asset-2", OwnerID: testUser, Model: ptr("M2")},
		}
		h := newHarness(t, 30, func() time.Time { return testNow }, seedAssets, nil, nil)

		if _, err := h.svc.Merge(testCtx(), "asset-1", "asset-2"); err != nil {
			t.Fatalf("Merge returned error %v, want nil", err)
		}
		dup, ok := h.ff.assetRepo.get("asset-2")
		if !ok {
			t.Fatalf("asset-2 not found after merge (must be retained, soft-deleted)")
		}
		if dup.MergedInto == nil || *dup.MergedInto != "asset-1" {
			t.Fatalf("duplicate MergedInto = %v, want %q", dup.MergedInto, "asset-1")
		}
		if dup.MergedAt == nil {
			t.Fatalf("duplicate MergedAt = nil, want non-nil")
		}
		if dup.DeletedAt == nil {
			t.Fatalf("duplicate DeletedAt = nil, want non-nil (soft-deleted)")
		}
		active, err := h.ff.assetRepo.List(testCtx(), repo.Owner(testUser), repo.Where("deleted_at", "IS NULL", nil))
		if err != nil {
			t.Fatalf("active List returned error %v, want nil", err)
		}
		ids := make([]string, 0, len(active))
		for _, a := range active {
			ids = append(ids, a.ID)
		}
		if len(ids) != 1 || ids[0] != "asset-1" {
			t.Fatalf("active list = %v, want [asset-1] (duplicate excluded)", ids)
		}
	})

	t.Run("survivor detail lists the merged-in asset + timestamp", func(t *testing.T) {
		t.Parallel()
		seedAssets := []entity.Asset{
			{ID: "asset-1", OwnerID: testUser, Model: ptr("M1")},
			{ID: "asset-2", OwnerID: testUser, Model: ptr("M2")},
		}
		h := newHarness(t, 30, func() time.Time { return testNow }, seedAssets, nil, nil)

		if _, err := h.svc.Merge(testCtx(), "asset-1", "asset-2"); err != nil {
			t.Fatalf("Merge returned error %v, want nil", err)
		}
		merged, err := h.svc.MergedAssets(testCtx(), "asset-1")
		if err != nil {
			t.Fatalf("MergedAssets returned error %v, want nil", err)
		}
		if len(merged) != 1 {
			t.Fatalf("MergedAssets len = %d, want 1", len(merged))
		}
		if merged[0].ID != "asset-2" {
			t.Fatalf("merged asset ID = %q, want asset-2", merged[0].ID)
		}
		if merged[0].MergedInto == nil || *merged[0].MergedInto != "asset-1" {
			t.Fatalf("merged asset MergedInto = %v, want %q", merged[0].MergedInto, "asset-1")
		}
		if merged[0].MergedAt == nil || !merged[0].MergedAt.Equal(testNow) {
			t.Fatalf("merged asset MergedAt = %v, want %v", merged[0].MergedAt, testNow)
		}
	})

	t.Run("unknown survivor returns ErrNotFound", func(t *testing.T) {
		t.Parallel()
		seedAssets := []entity.Asset{
			{ID: "asset-2", OwnerID: testUser, Model: ptr("M2")},
		}
		h := newHarness(t, 30, func() time.Time { return testNow }, seedAssets, nil, nil)

		before := h.ff.assetRepo.count()
		if _, err := h.svc.Merge(testCtx(), "nope", "asset-2"); !errors.Is(err, repo.ErrNotFound) {
			t.Fatalf("Merge error = %v, want errors.Is repo.ErrNotFound", err)
		}
		if h.ff.assetRepo.count() != before {
			t.Fatalf("asset count = %d, want %d (unchanged)", h.ff.assetRepo.count(), before)
		}
	})

	t.Run("unknown duplicate returns ErrNotFound", func(t *testing.T) {
		t.Parallel()
		seedAssets := []entity.Asset{
			{ID: "asset-1", OwnerID: testUser, Model: ptr("M1")},
		}
		h := newHarness(t, 30, func() time.Time { return testNow }, seedAssets, nil, nil)

		if _, err := h.svc.Merge(testCtx(), "asset-1", "nope"); !errors.Is(err, repo.ErrNotFound) {
			t.Fatalf("Merge error = %v, want errors.Is repo.ErrNotFound", err)
		}
	})

	t.Run("other-owner survivor returns ErrNotFound", func(t *testing.T) {
		t.Parallel()
		seedAssets := []entity.Asset{
			{ID: "asset-1", OwnerID: otherUser, Model: ptr("M1")},
			{ID: "asset-2", OwnerID: testUser, Model: ptr("M2")},
		}
		h := newHarness(t, 30, func() time.Time { return testNow }, seedAssets, nil, nil)

		if _, err := h.svc.Merge(testCtx(), "asset-1", "asset-2"); !errors.Is(err, repo.ErrNotFound) {
			t.Fatalf("Merge error = %v, want errors.Is repo.ErrNotFound", err)
		}
	})

	t.Run("other-owner duplicate returns ErrNotFound", func(t *testing.T) {
		t.Parallel()
		seedAssets := []entity.Asset{
			{ID: "asset-1", OwnerID: testUser, Model: ptr("M1")},
			{ID: "asset-2", OwnerID: otherUser, Model: ptr("M2")},
		}
		h := newHarness(t, 30, func() time.Time { return testNow }, seedAssets, nil, nil)

		if _, err := h.svc.Merge(testCtx(), "asset-1", "asset-2"); !errors.Is(err, repo.ErrNotFound) {
			t.Fatalf("Merge error = %v, want errors.Is repo.ErrNotFound", err)
		}
	})

	t.Run("no user in context returns ErrNoUser", func(t *testing.T) {
		t.Parallel()
		seedAssets := []entity.Asset{
			{ID: "asset-1", OwnerID: testUser, Model: ptr("M1")},
			{ID: "asset-2", OwnerID: testUser, Model: ptr("M2")},
		}
		h := newHarness(t, 30, func() time.Time { return testNow }, seedAssets, nil, nil)

		if _, err := h.svc.Merge(context.Background(), "asset-1", "asset-2"); !errors.Is(err, user.ErrNoUser) {
			t.Fatalf("Merge error = %v, want errors.Is user.ErrNoUser", err)
		}
	})

	t.Run("self-merge returns ErrInvalidMerge", func(t *testing.T) {
		t.Parallel()
		seedAssets := []entity.Asset{
			{ID: "asset-1", OwnerID: testUser, Model: ptr("M1")},
		}
		h := newHarness(t, 30, func() time.Time { return testNow }, seedAssets, nil, nil)

		if _, err := h.svc.Merge(testCtx(), "asset-1", "asset-1"); !errors.Is(err, ErrInvalidMerge) {
			t.Fatalf("Merge error = %v, want errors.Is ErrInvalidMerge", err)
		}
	})

	t.Run("soft-deleted survivor returns ErrConflict", func(t *testing.T) {
		t.Parallel()
		seedAssets := []entity.Asset{
			{ID: "asset-1", OwnerID: testUser, Model: ptr("M1"), DeletedAt: ptr(testNow)},
			{ID: "asset-2", OwnerID: testUser, Model: ptr("M2")},
		}
		h := newHarness(t, 30, func() time.Time { return testNow }, seedAssets, nil, nil)

		if _, err := h.svc.Merge(testCtx(), "asset-1", "asset-2"); !errors.Is(err, ErrConflict) {
			t.Fatalf("Merge error = %v, want errors.Is ErrConflict", err)
		}
		dup, ok := h.ff.assetRepo.get("asset-2")
		if !ok {
			t.Fatalf("asset-2 not found")
		}
		if dup.MergedInto != nil {
			t.Fatalf("duplicate MergedInto = %v, want nil (unchanged)", *dup.MergedInto)
		}
		if dup.DeletedAt != nil {
			t.Fatalf("duplicate DeletedAt = %v, want nil (unchanged)", *dup.DeletedAt)
		}
	})
}

// ---------------------------------------------------------------------------
// TestMergedAssets
// ---------------------------------------------------------------------------

func TestMergedAssets(t *testing.T) {
	t.Parallel()

	t.Run("empty when nothing merged", func(t *testing.T) {
		t.Parallel()
		seedAssets := []entity.Asset{
			{ID: "asset-1", OwnerID: testUser, Model: ptr("M1")},
		}
		h := newHarness(t, 30, func() time.Time { return testNow }, seedAssets, nil, nil)

		merged, err := h.svc.MergedAssets(testCtx(), "asset-1")
		if err != nil {
			t.Fatalf("MergedAssets returned error %v, want nil", err)
		}
		if len(merged) != 0 {
			t.Fatalf("MergedAssets len = %d, want 0", len(merged))
		}
	})

	t.Run("returns merged duplicates with timestamps", func(t *testing.T) {
		t.Parallel()
		seedAssets := []entity.Asset{
			{ID: "asset-1", OwnerID: testUser, Model: ptr("M1")},
			{ID: "asset-2", OwnerID: testUser, Model: ptr("M2"), MergedInto: ptr("asset-1"), MergedAt: ptr(testNow), DeletedAt: ptr(testNow)},
		}
		h := newHarness(t, 30, func() time.Time { return testNow }, seedAssets, nil, nil)

		merged, err := h.svc.MergedAssets(testCtx(), "asset-1")
		if err != nil {
			t.Fatalf("MergedAssets returned error %v, want nil", err)
		}
		if len(merged) != 1 {
			t.Fatalf("MergedAssets len = %d, want 1", len(merged))
		}
		if merged[0].ID != "asset-2" {
			t.Fatalf("merged asset ID = %q, want asset-2", merged[0].ID)
		}
		if merged[0].MergedAt == nil || !merged[0].MergedAt.Equal(testNow) {
			t.Fatalf("merged asset MergedAt = %v, want %v", merged[0].MergedAt, testNow)
		}
	})
}
