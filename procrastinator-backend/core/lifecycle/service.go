// Package lifecycle implements the asset lifecycle bounded context: it holds
// the soft-delete / restore / purge operations for owner-owned assets. A
// soft delete records a deleted_at timestamp without removing the row; the
// asset is restored within a fixed retention window and is purged (hard
// deleted) once it is older than the window. Detaching documents keeps their
// rows while nulling the asset_id link. Every method is user-scoped: it
// resolves the bound user from the context first and fails closed
// (user.ErrNoUser) without touching any repository when the user is absent.
package lifecycle

import (
	"context"
	"errors"
	"time"

	"procrastinator-backend/commons/entity"
	"procrastinator-backend/commons/repo"
	"procrastinator-backend/commons/user"
)

// ErrConflict is returned when attempting to restore an asset that is beyond
// the retention window. The API layer maps this sentinel to HTTP 409.
var ErrConflict = errors.New("asset is beyond the retention window and can no longer be restored")

// Service is the asset-lifecycle application service. It is user-scoped: every
// method resolves the user from the context first and fails closed
// (user.ErrNoUser) without touching any repository when the user is absent.
type Service struct {
	factory       *repo.Factory
	retentionDays int
	now           func() time.Time
}

// New constructs a Service over the given repository factory. retentionDays is
// the length of the retention window: a soft-deleted asset may be restored
// while its deleted_at is within retentionDays days of now, and is purged once
// it is older than the window.
func New(factory *repo.Factory, retentionDays int) *Service {
	return &Service{factory: factory, retentionDays: retentionDays, now: time.Now}
}

// retention returns the retention window as a duration.
func (s *Service) retention() time.Duration {
	return time.Duration(s.retentionDays) * 24 * time.Hour
}

// SoftDelete marks the given asset as soft-deleted by stamping deleted_at. The
// row is retained (not physically removed) and its documents stay linked. The
// operation is idempotent: an already-deleted asset is returned unchanged with
// no write.
func (s *Service) SoftDelete(ctx context.Context, assetID string) (entity.Asset, error) {
	tid, err := user.UserFrom(ctx)
	if err != nil {
		return entity.Asset{}, err
	}

	var updated entity.Asset
	err = s.factory.InTx(ctx, func(ctx context.Context, r *repo.Repos) error {
		asset, err := r.Assets.Get(ctx, assetID, repo.Owner(tid))
		if err != nil {
			return err
		}
		if asset.DeletedAt != nil {
			updated = asset
			return nil
		}
		now := s.now().UTC()
		asset.DeletedAt = &now
		updated, err = r.Assets.Update(ctx, asset, repo.Owner(tid))
		return err
	})
	if err != nil {
		return entity.Asset{}, err
	}
	return updated, nil
}

// Restore clears the soft-delete (deleted_at) on the given asset so it becomes
// active again. An asset that is not soft-deleted is returned unchanged. An
// asset beyond the retention window returns ErrConflict and is left untouched.
func (s *Service) Restore(ctx context.Context, assetID string) (entity.Asset, error) {
	tid, err := user.UserFrom(ctx)
	if err != nil {
		return entity.Asset{}, err
	}

	var updated entity.Asset
	err = s.factory.InTx(ctx, func(ctx context.Context, r *repo.Repos) error {
		asset, err := r.Assets.Get(ctx, assetID, repo.Owner(tid))
		if err != nil {
			return err
		}
		if asset.DeletedAt == nil {
			updated = asset
			return nil
		}
		if s.now().UTC().Sub(*asset.DeletedAt) > s.retention() {
			return ErrConflict
		}
		asset.DeletedAt = nil
		updated, err = r.Assets.Update(ctx, asset, repo.Owner(tid), repo.Set("deleted_at", nil))
		return err
	})
	if err != nil {
		return entity.Asset{}, err
	}
	return updated, nil
}

// PurgeExpired hard-deletes every soft-deleted asset of the bound user whose
// deleted_at is older than the retention window, nulling the asset_id link on
// each asset's documents while retaining the document rows. Sources are never
// touched. It returns the number of assets purged.
func (s *Service) PurgeExpired(ctx context.Context) (int, error) {
	tid, err := user.UserFrom(ctx)
	if err != nil {
		return 0, err
	}

	cutoff := s.now().UTC().Add(-s.retention())
	purged := 0
	err = s.factory.InTx(ctx, func(ctx context.Context, r *repo.Repos) error {
		expired, err := r.Assets.List(ctx, repo.Owner(tid), repo.Where("deleted_at", "<", cutoff))
		if err != nil {
			return err
		}
		for _, a := range expired {
			if _, err := repo.DetachDocumentsByAsset(ctx, r.Documents, a.ID, repo.Owner(tid)); err != nil {
				return err
			}
			if err := r.Assets.Delete(ctx, a.ID, repo.Owner(tid)); err != nil {
				return err
			}
			purged++
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return purged, nil
}
