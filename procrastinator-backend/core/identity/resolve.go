package identity

import (
	"context"
	"errors"

	"procrastinator-backend/commons"
	"procrastinator-backend/commons/entity"
	"procrastinator-backend/commons/repo"
	"procrastinator-backend/commons/user"
)

// ErrNoIdentity is returned when the extraction contains no usable
// identity fields (no serial, brand, or model).
var ErrNoIdentity = errors.New("no usable identity fields in extraction")

// DefaultCandidateLimit is the per-stage candidate bound used when a caller
// passes 0/negative.
const DefaultCandidateLimit = 10

// Sentinel errors returned by Resolve for non-commit outcomes. The real
// callers use Match to inspect the candidate set and hold/surface; these
// guard the direct Resolve write path.
var (
	// ErrAmbiguous signals multiple active candidates held for review.
	ErrAmbiguous = errors.New("ambiguous match: multiple candidates, held for review")
	// ErrSoftDeleted signals the only match is a soft-deleted asset; not
	// created (offer restore).
	ErrSoftDeleted = errors.New("match is a soft-deleted asset; not created (offer restore)")
)

// matchKind is the unexported outcome kind of the 3-stage lookup.
type matchKind int

// Exported outcome kinds.
const (
	// MatchUnresolved means no candidate at any stage.
	MatchUnresolved matchKind = iota
	// MatchMerge means exactly 1 active candidate (any stage).
	MatchMerge
	// MatchAmbiguous means >1 active candidate at a non-serial stage.
	MatchAmbiguous
	// MatchSoftDeleted means no active candidate, but >=1 soft-deleted match.
	MatchSoftDeleted
)

// MatchResult describes the outcome of the 3-stage indexed lookup.
type MatchResult struct {
	Kind       matchKind
	AssetID    *string        // non-nil when Kind == MatchMerge
	Candidates []entity.Asset // active set for Merge/Ambiguous; deleted set for SoftDeleted
}

// Resolve matches an extraction against existing assets via a deterministic
// 3-stage indexed lookup (norm_serial, then norm_brand+norm_model, then
// norm_name+norm_model) and applies the write. The boolean result reports
// whether a new asset was created.
//
// The owner is extracted from ctx and applied to every repository call via
// repo.Owner(tid). The ownerHouseholdID param fences resolution to a scope: a
// non-nil value restricts matches to the given household, a nil value
// restricts them to personal (non-household) assets, so a household upload
// never merges into a personal asset (and vice versa). A newly-created asset
// is stamped with the same scope.
//
// Soft-deleted assets never match: a stage with only soft-deleted candidates
// yields MatchSoftDeleted rather than creating. candidateLimit bounds the
// candidate set per stage; values <= 0 use DefaultCandidateLimit.
func Resolve(ctx context.Context, assets repo.Repository[entity.Asset], ext entity.Extraction, ownerHouseholdID *string, candidateLimit int) (entity.Asset, bool, error) {
	tid, err := user.UserFrom(ctx)
	if err != nil {
		return entity.Asset{}, false, err
	}

	res, err := matchExtraction(ctx, assets, ext, tid, ownerHouseholdID, candidateLimit)
	if err != nil {
		return entity.Asset{}, false, err
	}

	switch res.Kind {
	case MatchMerge:
		// Load the matched asset fresh, scoped by owner+fence; fall back to
		// createNew when it is no longer visible.
		fence := scopeFence(ownerHouseholdID)
		getOpts := append([]repo.Option{repo.Owner(tid)}, fence...)
		existing, err := assets.Get(ctx, *res.AssetID, getOpts...)
		if err == nil {
			merged, _, err := mergeInto(ctx, assets, tid, ext, existing)
			return merged, false, err
		}
		if !errors.Is(err, repo.ErrNotFound) {
			return entity.Asset{}, false, err
		}
		return createNew(ctx, assets, tid, ext, fence)
	case MatchAmbiguous:
		return entity.Asset{}, false, ErrAmbiguous
	case MatchSoftDeleted:
		return entity.Asset{}, false, ErrSoftDeleted
	default: // MatchUnresolved
		created, _, err := createNew(ctx, assets, tid, ext, scopeFence(ownerHouseholdID))
		return created, true, err
	}
}

// Match runs the 3-stage indexed lookup read-only and returns a MatchResult.
// It applies the same identity hierarchy and scope fence as Resolve without
// writing. candidateLimit bounds each stage's candidate set; values <= 0 use
// DefaultCandidateLimit. It returns ErrNoIdentity when no stage is eligible
// (no serial, and not brand+model, and not name+model).
func Match(ctx context.Context, assets repo.Repository[entity.Asset], ext entity.Extraction, ownerHouseholdID *string, candidateLimit int) (MatchResult, error) {
	tid, err := user.UserFrom(ctx)
	if err != nil {
		return MatchResult{}, err
	}
	return matchExtraction(ctx, assets, ext, tid, ownerHouseholdID, candidateLimit)
}

// scopeFence builds the scope-filtering options for ownerHouseholdID: a
// non-nil value restricts to that household, nil restricts to personal
// (owner_household_id IS NULL) assets.
func scopeFence(ownerHouseholdID *string) []repo.Option {
	if ownerHouseholdID != nil {
		return []repo.Option{repo.Where("owner_household_id", "=", *ownerHouseholdID)}
	}
	return []repo.Option{repo.Where("owner_household_id", "IS NULL", nil)}
}

// scopeFromFence recovers the owner_household_id scope from the fence options.
// A fence carrying "IS NULL" yields nil (personal); a fence carrying "=" on a
// household ID yields that household ID. An empty fence yields nil.
func scopeFromFence(fence []repo.Option) *string {
	for _, opt := range fence {
		o := repo.ApplyOptions(opt)
		for _, f := range o.Filters {
			if f.Field != "owner_household_id" {
				continue
			}
			if f.Op == "IS NULL" {
				return nil
			}
			if f.Op == "=" {
				if v, ok := f.Value.(string); ok {
					return &v
				}
			}
		}
	}
	return nil
}

// stage is one eligible lookup stage: a set of identity filters plus a flag
// indicating whether it is the serial stage (which always short-circuits to a
// single merge when it has ≥1 active candidate).
type stage struct {
	filters []repo.Option
	serial  bool
}

// normIdentity normalizes the extraction identity fields. Each returned value
// is empty (not a pointer) when its source field is nil/blank. ns, nb, nm, nn
// are the normalized serial, brand, model, and name respectively.
func normIdentity(ext entity.Extraction) (ns, nb, nm, nn string) {
	if ext.SerialNumber != nil {
		ns = commons.NormalizeSerial(*ext.SerialNumber)
	}
	if ext.Brand != nil {
		nb = commons.NormalizeName(*ext.Brand)
	}
	if ext.Model != nil {
		nm = commons.NormalizeName(*ext.Model)
	}
	if ext.Name != nil && *ext.Name != "" {
		nn = commons.NormalizeName(*ext.Name)
	}
	return ns, nb, nm, nn
}

// matchExtraction runs the ordered 3-stage indexed lookup and returns the
// first short-circuiting outcome. Stages run in order (serial, brand+model,
// name+model); the first stage returning ≥1 active candidate short-circuits
// the rest. Soft-deleted candidates never match but are surfaced when they are
// the only candidates at a stage.
func matchExtraction(
	ctx context.Context,
	assets repo.Repository[entity.Asset],
	ext entity.Extraction,
	tid string,
	ownerHouseholdID *string,
	candidateLimit int,
) (MatchResult, error) {
	ns, nb, nm, nn := normIdentity(ext)

	limit := candidateLimit
	if limit <= 0 {
		limit = DefaultCandidateLimit
	}

	var stages []stage
	if ns != "" {
		stages = append(stages, stage{filters: []repo.Option{repo.Where("norm_serial", "=", ns)}, serial: true})
	}
	if nb != "" && nm != "" {
		stages = append(stages, stage{filters: []repo.Option{repo.Where("norm_brand", "=", nb), repo.Where("norm_model", "=", nm)}})
	}
	if nn != "" && nm != "" {
		stages = append(stages, stage{filters: []repo.Option{repo.Where("norm_name", "=", nn), repo.Where("norm_model", "=", nm)}})
	}
	if len(stages) == 0 {
		return MatchResult{}, ErrNoIdentity
	}

	fence := scopeFence(ownerHouseholdID)

	for _, st := range stages {
		opts := append(
			[]repo.Option{repo.Owner(tid), repo.Limit(limit), repo.OrderBy("id")},
			st.filters...,
		)
		opts = append(opts, fence...)

		results, err := assets.List(ctx, opts...)
		if err != nil {
			return MatchResult{}, err
		}

		active := make([]entity.Asset, 0, len(results))
		deleted := make([]entity.Asset, 0, len(results))
		for _, a := range results {
			if a.DeletedAt == nil {
				active = append(active, a)
			} else {
				deleted = append(deleted, a)
			}
		}

		if len(active) > 0 {
			if st.serial || len(active) == 1 {
				id := active[0].ID
				return MatchResult{Kind: MatchMerge, AssetID: &id, Candidates: active}, nil
			}
			return MatchResult{Kind: MatchAmbiguous, Candidates: active}, nil
		}
		if len(deleted) > 0 {
			return MatchResult{Kind: MatchSoftDeleted, Candidates: deleted}, nil
		}
		// No candidate at this stage; continue to the next stage.
	}

	return MatchResult{Kind: MatchUnresolved}, nil
}

// CommitCandidate commits a stored candidate: if matchedAssetID is non-nil it
// loads the target asset and merges the extraction into it (falling back to
// createNew when the asset no longer exists); if nil it creates a new asset.
// Reuses the existing mergeInto / createNew / newAsset / scopeFence logic.
func CommitCandidate(ctx context.Context, assets repo.Repository[entity.Asset], ext entity.Extraction, matchedAssetID *string, ownerHouseholdID *string) (entity.Asset, error) {
	tid, err := user.UserFrom(ctx)
	if err != nil {
		return entity.Asset{}, err
	}

	fence := scopeFence(ownerHouseholdID)

	if matchedAssetID != nil {
		getOpts := append([]repo.Option{repo.Owner(tid)}, fence...)
		existing, err := assets.Get(ctx, *matchedAssetID, getOpts...)
		if err == nil {
			merged, _, err := mergeInto(ctx, assets, tid, ext, existing)
			return merged, err
		}
		if !errors.Is(err, repo.ErrNotFound) {
			return entity.Asset{}, err
		}
		// Asset was deleted or is no longer visible: fall through to createNew.
	}

	created, err := assets.Create(ctx, newAsset(ext, fence), repo.Owner(tid))
	return created, err
}

// createNew builds a fresh asset from the extraction and stores it.
func createNew(ctx context.Context, assets repo.Repository[entity.Asset], tid string, ext entity.Extraction, fence []repo.Option) (entity.Asset, bool, error) {
	created, err := assets.Create(ctx, newAsset(ext, fence), repo.Owner(tid))
	if err != nil {
		return entity.Asset{}, false, err
	}
	return created, true, nil
}

// newAsset builds an entity.Asset for creation from the extraction. Norm
// columns are nil when the corresponding normalized value is empty. Name and
// its normalized form are always stamped (when Name is present) so the
// name+model stage can match this asset later. The owner_household_id scope is
// stamped from the fence (nil for a personal asset).
func newAsset(ext entity.Extraction, fence []repo.Option) entity.Asset {
	ns, nb, nm, nn := normIdentity(ext)

	var normSerialP, normBrandP, normModelP, normNameP *string
	if ns != "" {
		normSerialP = &ns
	}
	if nb != "" {
		normBrandP = &nb
	}
	if nm != "" {
		normModelP = &nm
	}
	if nn != "" {
		normNameP = &nn
	}
	return entity.Asset{
		Brand:              ext.Brand,
		Model:              ext.Model,
		Name:               ext.Name,
		SerialNumber:       ext.SerialNumber,
		NormSerial:         normSerialP,
		NormBrand:          normBrandP,
		NormModel:          normModelP,
		NormName:           normNameP,
		AssetCategory:      ext.AssetCategory,
		CategoryConfidence: ext.CategoryConfidence,
		PurchaseDate:       ext.PurchaseDate,
		WarrantyEnd:        ext.WarrantyEnd,
		Price:              ext.Price,
		Currency:           ext.Currency,
		Metadata:           ext.Metadata,
		OwnerHouseholdID:   scopeFromFence(fence),
		Confidence:         ext.Confidence,
	}
}

// mergeInto applies the non-nil extraction fields onto a copy of the existing
// asset and persists the full entity.
func mergeInto(ctx context.Context, assets repo.Repository[entity.Asset], tid string, ext entity.Extraction, existing entity.Asset) (entity.Asset, bool, error) {
	updated := existing
	if ext.Brand != nil {
		updated.Brand = ext.Brand
	}
	if ext.Model != nil {
		updated.Model = ext.Model
	}
	if ext.Name != nil {
		updated.Name = ext.Name
	}
	if ext.SerialNumber != nil {
		updated.SerialNumber = ext.SerialNumber
	}
	if ext.PurchaseDate != nil {
		updated.PurchaseDate = ext.PurchaseDate
	}
	if ext.WarrantyEnd != nil {
		updated.WarrantyEnd = ext.WarrantyEnd
	}
	if ext.Price != nil {
		updated.Price = ext.Price
	}
	if ext.Currency != nil {
		updated.Currency = ext.Currency
	}
	if ext.Metadata != nil {
		updated.Metadata = mergeMetadata(existing.Metadata, ext.Metadata)
	}
	if ext.Confidence != nil {
		updated.Confidence = ext.Confidence
	}

	// Category merge: user-set categories are sticky; inferred categories
	// only move to a strictly higher-confidence inference.
	if ext.AssetCategory != nil {
		if categoryShouldReplace(existing, ext.CategoryConfidence) {
			updated.AssetCategory = ext.AssetCategory
			if ext.CategoryConfidence != nil {
				updated.CategoryConfidence = ext.CategoryConfidence
			}
		}
	}

	merged, err := assets.Update(ctx, updated, repo.Owner(tid))
	if err != nil {
		return entity.Asset{}, false, err
	}
	return merged, false, nil
}

// mergeMetadata shallow-merges incoming metadata over existing metadata.
// A nil incoming map yields nil (no metadata update).
func mergeMetadata(existing, incoming map[string]any) map[string]any {
	if incoming == nil {
		return nil
	}
	merged := make(map[string]any, len(existing)+len(incoming))
	for k, v := range existing {
		merged[k] = v
	}
	for k, v := range incoming {
		merged[k] = v
	}
	return merged
}

// categoryShouldReplace decides whether an incoming asset category (with its
// confidence) should overwrite an existing asset's category during a merge.
// A user-corrected category (CategoryUserSet) is sticky and never replaced.
// When the asset has no category yet, the incoming category is always adopted.
// Otherwise the incoming category replaces the existing one only when its
// confidence is strictly higher; nil confidences are treated as 0.0.
func categoryShouldReplace(existing entity.Asset, incomingConf *float64) bool {
	if existing.CategoryUserSet {
		return false
	}
	if existing.AssetCategory == nil {
		return true
	}
	oldConf := 0.0
	if existing.CategoryConfidence != nil {
		oldConf = *existing.CategoryConfidence
	}
	newConf := 0.0
	if incomingConf != nil {
		newConf = *incomingConf
	}
	return newConf > oldConf
}
