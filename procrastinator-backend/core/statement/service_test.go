package statement

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"procrastinator-backend/commons/entity"
	"procrastinator-backend/commons/repo"
	"procrastinator-backend/commons/tenant"
)

// testTenant and testTenantB are the two tenant IDs used across the service
// tests; foreign-tenant scenarios cross between them.
const (
	testTenant  = "test-tenant"
	testTenantB = "other-tenant"
)

// Fixed base times so that List-ordering assertions are deterministic and no
// real clock is read by the tests.
var (
	fixedT0   = time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC)
	fixedT1   = time.Date(2026, 8, 21, 9, 0, 0, 0, time.UTC)
	fixedT2   = time.Date(2026, 8, 22, 9, 0, 0, 0, time.UTC)
	fixedDay  = time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC)
	fixedDay1 = time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC)
)

// Test sentinels shared by multiple cases.
var (
	errStoreUnsupported = errors.New("unsupported statement type")
	errMoveCreateFail   = errors.New("movement create failed")
)

// Compile-time interface guards.
var (
	_ repo.Repository[entity.FinancialAccount] = (*fakeAccountRepo)(nil)
	_ repo.Repository[entity.Source]           = (*fakeSourceRepo)(nil)
	_ repo.Repository[entity.MoneyMovement]    = (*fakeMovementRepo)(nil)
	_ repo.Repository[entity.ImportBatch]      = (*fakeImportBatchRepo)(nil)
	_ repo.Repository[entity.ImportLine]       = (*fakeImportLineRepo)(nil)
	_ StatementSourceStore                     = (*fakeSourceStore)(nil)
	_ MovementsForAccountLister                = (*fakeMovLister)(nil)
	_ LinkCandidateLister                      = (*fakeLinkLister)(nil)
	_ PDFTextExtractor                         = (*fakePDF)(nil)
)

// --- fakeAccountRepo --------------------------------------------------------

// fakeAccountRepo is an in-memory implementation of
// repo.Repository[entity.FinancialAccount] with tenant-scoped Get.
type fakeAccountRepo struct {
	mu        sync.Mutex
	accounts  map[string]entity.FinancialAccount
	nextID    int
	createErr error
}

func newFakeAccountRepo() *fakeAccountRepo {
	return &fakeAccountRepo{accounts: make(map[string]entity.FinancialAccount)}
}

func (r *fakeAccountRepo) Get(_ context.Context, id string, opts ...repo.Option) (entity.FinancialAccount, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.accounts[id]
	if !ok {
		return entity.FinancialAccount{}, repo.ErrNotFound
	}
	if tid := tenantFromOpts(opts); tid != "" && a.TenantID != tid {
		return entity.FinancialAccount{}, repo.ErrNotFound
	}
	return a, nil
}

func (r *fakeAccountRepo) List(_ context.Context, opts ...repo.Option) ([]entity.FinancialAccount, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	o := repo.ApplyOptions(opts...)
	out := make([]entity.FinancialAccount, 0, len(r.accounts))
	for _, a := range r.accounts {
		if o.TenantID != "" && a.TenantID != o.TenantID {
			continue
		}
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (r *fakeAccountRepo) Create(_ context.Context, a entity.FinancialAccount, opts ...repo.Option) (entity.FinancialAccount, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.createErr != nil {
		return entity.FinancialAccount{}, r.createErr
	}
	if a.ID == "" {
		r.nextID++
		a.ID = "acct-" + strconv.Itoa(r.nextID)
	}
	if tid := tenantFromOpts(opts); tid != "" {
		a.TenantID = tid
	}
	r.accounts[a.ID] = a
	return a, nil
}

func (r *fakeAccountRepo) Update(_ context.Context, a entity.FinancialAccount, opts ...repo.Option) (entity.FinancialAccount, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.accounts[a.ID]; !ok {
		return entity.FinancialAccount{}, repo.ErrNotFound
	}
	if tid := tenantFromOpts(opts); tid != "" {
		a.TenantID = tid
	}
	r.accounts[a.ID] = a
	return a, nil
}

func (r *fakeAccountRepo) Delete(_ context.Context, id string, _ ...repo.Option) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.accounts[id]; !ok {
		return repo.ErrNotFound
	}
	delete(r.accounts, id)
	return nil
}

// seed inserts an account directly (bypassing Create) for test fixtures.
func (r *fakeAccountRepo) seed(a entity.FinancialAccount) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.accounts[a.ID] = a
}

// get returns the account with the given ID.
func (r *fakeAccountRepo) get(id string) (entity.FinancialAccount, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.accounts[id]
	return a, ok
}

// count returns the number of stored accounts.
func (r *fakeAccountRepo) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.accounts)
}

// --- fakeSourceRepo ----------------------------------------------------------

// fakeSourceRepo is an in-memory implementation of
// repo.Repository[entity.Source] with tenant-scoped Get.
type fakeSourceRepo struct {
	mu        sync.Mutex
	sources   map[string]entity.Source
	nextID    int
	createErr error
}

func newFakeSourceRepo() *fakeSourceRepo {
	return &fakeSourceRepo{sources: make(map[string]entity.Source)}
}

func (r *fakeSourceRepo) Get(_ context.Context, id string, opts ...repo.Option) (entity.Source, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.sources[id]
	if !ok {
		return entity.Source{}, repo.ErrNotFound
	}
	if tid := tenantFromOpts(opts); tid != "" && s.TenantID != tid {
		return entity.Source{}, repo.ErrNotFound
	}
	return s, nil
}

func (r *fakeSourceRepo) List(_ context.Context, opts ...repo.Option) ([]entity.Source, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	o := repo.ApplyOptions(opts...)
	out := make([]entity.Source, 0, len(r.sources))
	for _, s := range r.sources {
		if o.TenantID != "" && s.TenantID != o.TenantID {
			continue
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (r *fakeSourceRepo) Create(_ context.Context, s entity.Source, opts ...repo.Option) (entity.Source, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.createErr != nil {
		return entity.Source{}, r.createErr
	}
	if s.ID == "" {
		r.nextID++
		s.ID = "src-" + strconv.Itoa(r.nextID)
	}
	if tid := tenantFromOpts(opts); tid != "" {
		s.TenantID = tid
	}
	r.sources[s.ID] = s
	return s, nil
}

func (r *fakeSourceRepo) Update(_ context.Context, s entity.Source, opts ...repo.Option) (entity.Source, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.sources[s.ID]; !ok {
		return entity.Source{}, repo.ErrNotFound
	}
	if tid := tenantFromOpts(opts); tid != "" {
		s.TenantID = tid
	}
	r.sources[s.ID] = s
	return s, nil
}

func (r *fakeSourceRepo) Delete(_ context.Context, id string, _ ...repo.Option) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.sources[id]; !ok {
		return repo.ErrNotFound
	}
	delete(r.sources, id)
	return nil
}

// seed inserts a source directly (bypassing Create) for test fixtures.
func (r *fakeSourceRepo) seed(s entity.Source) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sources[s.ID] = s
}

// get returns the source with the given ID.
func (r *fakeSourceRepo) get(id string) (entity.Source, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.sources[id]
	return s, ok
}

// count returns the number of stored sources.
func (r *fakeSourceRepo) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.sources)
}

// byFilename returns the stored sources whose Filename equals filename and
// whose TenantID equals tenant (both must match).
func (r *fakeSourceRepo) byFilename(filename, tenant string) []entity.Source {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]entity.Source, 0, len(r.sources))
	for _, s := range r.sources {
		if s.Filename == filename && s.TenantID == tenant {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// --- fakeMovementRepo -------------------------------------------------------

// movementUpdateCall records one Update call for auto-link assertions.
type movementUpdateCall struct {
	movement entity.MoneyMovement
}

// fakeMovementRepo is an in-memory implementation of
// repo.Repository[entity.MoneyMovement]. List honors repo.Tenant, repo.Where
// ("=" on import_batch_id) and repo.OrderBy (empty, "id", or "created_at, id").
// Create is injectably failible on the Nth call for atomicity tests.
type fakeMovementRepo struct {
	mu        sync.Mutex
	movs      map[string]entity.MoneyMovement
	nextID    int
	createErr error // returned by every Create when set
	failAt    int   // fail Create on this 1-based call number (0 = never)
	calls     int   // 1-based count of Create calls
	saved     map[string]entity.MoneyMovement
	updates   []movementUpdateCall
}

func newFakeMovementRepo() *fakeMovementRepo {
	return &fakeMovementRepo{movs: make(map[string]entity.MoneyMovement)}
}

func (r *fakeMovementRepo) Get(_ context.Context, id string, opts ...repo.Option) (entity.MoneyMovement, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	m, ok := r.movs[id]
	if !ok {
		return entity.MoneyMovement{}, repo.ErrNotFound
	}
	if tid := tenantFromOpts(opts); tid != "" && m.TenantID != tid {
		return entity.MoneyMovement{}, repo.ErrNotFound
	}
	return m, nil
}

func (r *fakeMovementRepo) List(_ context.Context, opts ...repo.Option) ([]entity.MoneyMovement, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	o := repo.ApplyOptions(opts...)
	out := make([]entity.MoneyMovement, 0, len(r.movs))
	for _, m := range r.movs {
		if o.TenantID != "" && m.TenantID != o.TenantID {
			continue
		}
		match := true
		for _, f := range o.Filters {
			if f.Op != "=" {
				match = false
				break
			}
			if f.Field == "import_batch_id" {
				if m.ImportBatchID == nil || *m.ImportBatchID != asString(f.Value) {
					match = false
					break
				}
			}
		}
		if match {
			out = append(out, m)
		}
	}
	switch o.OrderBy {
	case "created_at, id":
		sort.Slice(out, func(i, j int) bool {
			if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
				return out[i].CreatedAt.Before(out[j].CreatedAt)
			}
			return out[i].ID < out[j].ID
		})
	default:
		sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	}
	return out, nil
}

func (r *fakeMovementRepo) Create(_ context.Context, m entity.MoneyMovement, opts ...repo.Option) (entity.MoneyMovement, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	if r.createErr != nil {
		return entity.MoneyMovement{}, r.createErr
	}
	if r.failAt > 0 && r.calls == r.failAt {
		return entity.MoneyMovement{}, errMoveCreateFail
	}
	if m.ID == "" {
		r.nextID++
		m.ID = "mv-" + strconv.Itoa(r.nextID)
	}
	if tid := tenantFromOpts(opts); tid != "" {
		m.TenantID = tid
	}
	if m.CreatedAt.IsZero() {
		m.CreatedAt = fixedT0.Add(time.Duration(r.nextID) * time.Second)
	}
	r.movs[m.ID] = m
	return m, nil
}

func (r *fakeMovementRepo) Update(_ context.Context, m entity.MoneyMovement, opts ...repo.Option) (entity.MoneyMovement, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.movs[m.ID]; !ok {
		return entity.MoneyMovement{}, repo.ErrNotFound
	}
	if tid := tenantFromOpts(opts); tid != "" {
		m.TenantID = tid
	}
	r.updates = append(r.updates, movementUpdateCall{movement: m})
	r.movs[m.ID] = m
	return m, nil
}

func (r *fakeMovementRepo) Delete(_ context.Context, id string, _ ...repo.Option) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.movs[id]; !ok {
		return repo.ErrNotFound
	}
	delete(r.movs, id)
	return nil
}

// seed inserts a movement directly (bypassing Create) for test fixtures.
func (r *fakeMovementRepo) seed(m entity.MoneyMovement) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if m.CreatedAt.IsZero() {
		m.CreatedAt = fixedT0
	}
	r.movs[m.ID] = m
}

// count returns the number of stored movements.
func (r *fakeMovementRepo) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.movs)
}

// all returns every stored movement in ID order.
func (r *fakeMovementRepo) all() []entity.MoneyMovement {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]entity.MoneyMovement, 0, len(r.movs))
	for _, m := range r.movs {
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// forBatch returns the movements whose ImportBatchID equals batchID, in
// CreatedAt/ID order (matching the List contract).
func (r *fakeMovementRepo) forBatch(batchID string) []entity.MoneyMovement {
	all := r.all()
	out := make([]entity.MoneyMovement, 0, len(all))
	for _, m := range all {
		if m.ImportBatchID != nil && *m.ImportBatchID == batchID {
			out = append(out, m)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.Before(out[j].CreatedAt)
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// updateCount returns the number of Update calls.
func (r *fakeMovementRepo) updateCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.updates)
}

// linkedDocument reports whether any stored movement is already linked to the
// given document ID (models the concrete NOT EXISTS in the link query).
func (r *fakeMovementRepo) linkedDocument(docID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, m := range r.movs {
		if m.LinkedDocumentID != nil && *m.LinkedDocumentID == docID {
			return true
		}
	}
	return false
}

// snapshot deep-copies the movement map for transaction rollback support.
func (r *fakeMovementRepo) snapshot() {
	r.mu.Lock()
	defer r.mu.Unlock()
	saved := make(map[string]entity.MoneyMovement, len(r.movs))
	for id, m := range r.movs {
		saved[id] = deepMovement(m)
	}
	r.saved = saved
}

// rollback restores the movement map from the last snapshot.
func (r *fakeMovementRepo) rollback() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.saved == nil {
		return
	}
	r.movs = make(map[string]entity.MoneyMovement, len(r.saved))
	for id, m := range r.saved {
		r.movs[id] = deepMovement(m)
	}
	r.saved = nil
}

// --- fakeImportBatchRepo ----------------------------------------------------

// fakeImportBatchRepo is an in-memory implementation of
// repo.Repository[entity.ImportBatch].
type fakeImportBatchRepo struct {
	mu        sync.Mutex
	batches   map[string]entity.ImportBatch
	nextID    int
	createErr error
	saved     map[string]entity.ImportBatch
}

func newFakeImportBatchRepo() *fakeImportBatchRepo {
	return &fakeImportBatchRepo{batches: make(map[string]entity.ImportBatch)}
}

func (r *fakeImportBatchRepo) Get(_ context.Context, id string, opts ...repo.Option) (entity.ImportBatch, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	b, ok := r.batches[id]
	if !ok {
		return entity.ImportBatch{}, repo.ErrNotFound
	}
	if tid := tenantFromOpts(opts); tid != "" && b.TenantID != tid {
		return entity.ImportBatch{}, repo.ErrNotFound
	}
	return b, nil
}

func (r *fakeImportBatchRepo) List(_ context.Context, opts ...repo.Option) ([]entity.ImportBatch, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	o := repo.ApplyOptions(opts...)
	out := make([]entity.ImportBatch, 0, len(r.batches))
	for _, b := range r.batches {
		if o.TenantID != "" && b.TenantID != o.TenantID {
			continue
		}
		out = append(out, b)
	}
	if o.OrderBy == "created_at, id" {
		sort.Slice(out, func(i, j int) bool {
			if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
				return out[i].CreatedAt.Before(out[j].CreatedAt)
			}
			return out[i].ID < out[j].ID
		})
	} else {
		sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	}
	return out, nil
}

func (r *fakeImportBatchRepo) Create(_ context.Context, b entity.ImportBatch, opts ...repo.Option) (entity.ImportBatch, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.createErr != nil {
		return entity.ImportBatch{}, r.createErr
	}
	if b.ID == "" {
		r.nextID++
		b.ID = "batch-" + strconv.Itoa(r.nextID)
	}
	if tid := tenantFromOpts(opts); tid != "" {
		b.TenantID = tid
	}
	if b.CreatedAt.IsZero() {
		b.CreatedAt = fixedT0.Add(time.Duration(r.nextID) * time.Second)
	}
	if b.UpdatedAt.IsZero() {
		b.UpdatedAt = b.CreatedAt
	}
	r.batches[b.ID] = b
	return b, nil
}

func (r *fakeImportBatchRepo) Update(_ context.Context, b entity.ImportBatch, opts ...repo.Option) (entity.ImportBatch, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.batches[b.ID]; !ok {
		return entity.ImportBatch{}, repo.ErrNotFound
	}
	if tid := tenantFromOpts(opts); tid != "" {
		b.TenantID = tid
	}
	r.batches[b.ID] = b
	return b, nil
}

func (r *fakeImportBatchRepo) Delete(_ context.Context, id string, _ ...repo.Option) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.batches[id]; !ok {
		return repo.ErrNotFound
	}
	delete(r.batches, id)
	return nil
}

// seed inserts a batch directly (bypassing Create) for test fixtures.
func (r *fakeImportBatchRepo) seed(b entity.ImportBatch) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if b.CreatedAt.IsZero() {
		b.CreatedAt = fixedT0
	}
	if b.UpdatedAt.IsZero() {
		b.UpdatedAt = b.CreatedAt
	}
	r.batches[b.ID] = b
}

// get returns the batch with the given ID.
func (r *fakeImportBatchRepo) get(id string) (entity.ImportBatch, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	b, ok := r.batches[id]
	return b, ok
}

// count returns the number of stored batches.
func (r *fakeImportBatchRepo) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.batches)
}

// snapshot deep-copies the batch map for transaction rollback support.
func (r *fakeImportBatchRepo) snapshot() {
	r.mu.Lock()
	defer r.mu.Unlock()
	saved := make(map[string]entity.ImportBatch, len(r.batches))
	for id, b := range r.batches {
		saved[id] = b
	}
	r.saved = saved
}

// rollback restores the batch map from the last snapshot.
func (r *fakeImportBatchRepo) rollback() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.saved == nil {
		return
	}
	r.batches = make(map[string]entity.ImportBatch, len(r.saved))
	for id, b := range r.saved {
		r.batches[id] = b
	}
	r.saved = nil
}

// --- fakeImportLineRepo -----------------------------------------------------

// fakeImportLineRepo is an in-memory implementation of
// repo.Repository[entity.ImportLine]. List honors repo.Tenant, repo.Where
// ("=" on batch_id) and repo.OrderBy ("line_ref").
type fakeImportLineRepo struct {
	mu        sync.Mutex
	lines     map[string]entity.ImportLine
	nextID    int
	createErr error
	saved     map[string]entity.ImportLine
}

func newFakeImportLineRepo() *fakeImportLineRepo {
	return &fakeImportLineRepo{lines: make(map[string]entity.ImportLine)}
}

func (r *fakeImportLineRepo) Get(_ context.Context, id string, opts ...repo.Option) (entity.ImportLine, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	l, ok := r.lines[id]
	if !ok {
		return entity.ImportLine{}, repo.ErrNotFound
	}
	if tid := tenantFromOpts(opts); tid != "" && l.TenantID != tid {
		return entity.ImportLine{}, repo.ErrNotFound
	}
	return l, nil
}

func (r *fakeImportLineRepo) List(_ context.Context, opts ...repo.Option) ([]entity.ImportLine, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	o := repo.ApplyOptions(opts...)
	out := make([]entity.ImportLine, 0, len(r.lines))
	for _, l := range r.lines {
		if o.TenantID != "" && l.TenantID != o.TenantID {
			continue
		}
		match := true
		for _, f := range o.Filters {
			if f.Op != "=" {
				match = false
				break
			}
			if f.Field == "batch_id" {
				if l.BatchID != asString(f.Value) {
					match = false
					break
				}
			}
		}
		if match {
			out = append(out, l)
		}
	}
	if o.OrderBy == "line_ref" {
		sort.Slice(out, func(i, j int) bool {
			if out[i].LineRef != out[j].LineRef {
				return out[i].LineRef < out[j].LineRef
			}
			return out[i].ID < out[j].ID
		})
	} else {
		sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	}
	return out, nil
}

func (r *fakeImportLineRepo) Create(_ context.Context, l entity.ImportLine, opts ...repo.Option) (entity.ImportLine, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.createErr != nil {
		return entity.ImportLine{}, r.createErr
	}
	if l.ID == "" {
		r.nextID++
		l.ID = "line-" + strconv.Itoa(r.nextID)
	}
	if tid := tenantFromOpts(opts); tid != "" {
		l.TenantID = tid
	}
	if l.CreatedAt.IsZero() {
		l.CreatedAt = fixedT0.Add(time.Duration(r.nextID) * time.Second)
	}
	r.lines[l.ID] = l
	return l, nil
}

func (r *fakeImportLineRepo) Update(_ context.Context, l entity.ImportLine, opts ...repo.Option) (entity.ImportLine, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.lines[l.ID]; !ok {
		return entity.ImportLine{}, repo.ErrNotFound
	}
	if tid := tenantFromOpts(opts); tid != "" {
		l.TenantID = tid
	}
	r.lines[l.ID] = l
	return l, nil
}

func (r *fakeImportLineRepo) Delete(_ context.Context, id string, _ ...repo.Option) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.lines[id]; !ok {
		return repo.ErrNotFound
	}
	delete(r.lines, id)
	return nil
}

// seed inserts a line directly (bypassing Create) for test fixtures.
func (r *fakeImportLineRepo) seed(l entity.ImportLine) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if l.CreatedAt.IsZero() {
		l.CreatedAt = fixedT0
	}
	r.lines[l.ID] = l
}

// forBatch returns the lines of a batch ordered by LineRef.
func (r *fakeImportLineRepo) forBatch(batchID string) []entity.ImportLine {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]entity.ImportLine, 0, len(r.lines))
	for _, l := range r.lines {
		if l.BatchID == batchID {
			out = append(out, l)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].LineRef != out[j].LineRef {
			return out[i].LineRef < out[j].LineRef
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// count returns the number of stored lines.
func (r *fakeImportLineRepo) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.lines)
}

// snapshot deep-copies the line map for transaction rollback support.
func (r *fakeImportLineRepo) snapshot() {
	r.mu.Lock()
	defer r.mu.Unlock()
	saved := make(map[string]entity.ImportLine, len(r.lines))
	for id, l := range r.lines {
		saved[id] = deepImportLine(l)
	}
	r.saved = saved
}

// rollback restores the line map from the last snapshot.
func (r *fakeImportLineRepo) rollback() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.saved == nil {
		return
	}
	r.lines = make(map[string]entity.ImportLine, len(r.saved))
	for id, l := range r.saved {
		r.lines[id] = deepImportLine(l)
	}
	r.saved = nil
}

// --- fakeSourceStore --------------------------------------------------------

// sourceStoreCall records one Store call.
type sourceStoreCall struct {
	name string
	data []byte
}

// fakeSourceStore is a configurable in-memory implementation of
// StatementSourceStore. It records every call and can either return an error
// (unsupported content) or a Source with a configurable ContentType (default
// text/csv) carrying the ORIGINAL filename.
type fakeSourceStore struct {
	mu          sync.Mutex
	calls       []sourceStoreCall
	err         error
	contentType string
	nextID      int
}

func newFakeSourceStore() *fakeSourceStore {
	return &fakeSourceStore{contentType: "text/csv"}
}

func (s *fakeSourceStore) Store(_ context.Context, originalName string, data []byte) (entity.Source, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	payload := make([]byte, len(data))
	copy(payload, data)
	s.calls = append(s.calls, sourceStoreCall{name: originalName, data: payload})
	if s.err != nil {
		return entity.Source{}, s.err
	}
	s.nextID++
	ct := s.contentType
	if ct == "" {
		ct = "text/csv"
	}
	return entity.Source{
		ID:          "src-" + strconv.Itoa(s.nextID),
		Filename:    originalName,
		ContentType: ct,
		Size:        int64(len(data)),
		UploadedAt:  fixedT0,
	}, nil
}

// callCount returns the number of Store calls.
func (s *fakeSourceStore) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.calls)
}

// lastCall returns the name and bytes of the most recent Store call.
func (s *fakeSourceStore) lastCall() (string, []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.calls) == 0 {
		return "", nil
	}
	last := s.calls[len(s.calls)-1]
	return last.name, last.data
}

// --- fakeMovLister ----------------------------------------------------------

// movListerCall records one MovementsForAccount call.
type movListerCall struct {
	accountID string
	opts      []repo.Option
}

// fakeMovLister is an in-memory implementation of MovementsForAccountLister.
// It filters seeded movements by tenant (from repo.Tenant in opts) AND by
// account (SourceAccountID or DestinationAccountID equals accountID), and
// records every call with its options.
type fakeMovLister struct {
	mu      sync.Mutex
	movs    []entity.MoneyMovement
	calls   []movListerCall
	listErr error
}

func newFakeMovLister() *fakeMovLister {
	return &fakeMovLister{}
}

// seed adds a movement to the lister's own seed set (not the fake repo).
func (l *fakeMovLister) seed(m entity.MoneyMovement) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.movs = append(l.movs, m)
}

func (l *fakeMovLister) MovementsForAccount(_ context.Context, accountID string, opts ...repo.Option) ([]entity.MoneyMovement, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	captured := make([]repo.Option, len(opts))
	copy(captured, opts)
	l.calls = append(l.calls, movListerCall{accountID: accountID, opts: captured})
	if l.listErr != nil {
		return nil, l.listErr
	}
	tid := tenantFromOpts(opts)
	out := make([]entity.MoneyMovement, 0, len(l.movs))
	for _, m := range l.movs {
		if m.TenantID != tid {
			continue
		}
		if (m.SourceAccountID != nil && *m.SourceAccountID == accountID) ||
			(m.DestinationAccountID != nil && *m.DestinationAccountID == accountID) {
			out = append(out, m)
		}
	}
	return out, nil
}

// lastCallOpts returns the options of the most recent call.
func (l *fakeMovLister) lastCallOpts() []repo.Option {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.calls) == 0 {
		return nil
	}
	return l.calls[len(l.calls)-1].opts
}

// callAccountID returns the account ID of the most recent call.
func (l *fakeMovLister) callAccountID() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.calls) == 0 {
		return ""
	}
	return l.calls[len(l.calls)-1].accountID
}

// --- fakeLinkLister ---------------------------------------------------------

// linkSeedDoc tags a document with the amount/currency the concrete store
// would match on, plus its tenant.
type linkSeedDoc struct {
	doc      entity.Document
	amount   string
	currency string
	tenant   string
}

// fakeLinkLister is an in-memory implementation of LinkCandidateLister. A
// seeded document is a candidate only when its tenant and amount/currency match
// the query AND it is not already linked to any movement in the movement repo
// (models the concrete NOT EXISTS). Calls are recorded.
type fakeLinkLister struct {
	mu      sync.Mutex
	seeds   []linkSeedDoc
	calls   []movListerCall
	err     error
	movRepo *fakeMovementRepo // consulted to model NOT EXISTS
}

func newFakeLinkLister(movRepo *fakeMovementRepo) *fakeLinkLister {
	return &fakeLinkLister{movRepo: movRepo}
}

// seed adds a candidate document.
func (l *fakeLinkLister) seed(doc entity.Document, amount, currency, tenant string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.seeds = append(l.seeds, linkSeedDoc{doc: doc, amount: amount, currency: currency, tenant: tenant})
}

func (l *fakeLinkLister) LinkCandidates(_ context.Context, amount, currency string, opts ...repo.Option) ([]entity.Document, error) {
	l.mu.Lock()
	captured := make([]repo.Option, len(opts))
	copy(captured, opts)
	l.calls = append(l.calls, movListerCall{accountID: amount + "|" + currency, opts: captured})
	seeds := make([]linkSeedDoc, len(l.seeds))
	copy(seeds, l.seeds)
	l.mu.Unlock()
	if l.err != nil {
		return nil, l.err
	}
	tid := tenantFromOpts(opts)
	out := make([]entity.Document, 0, len(seeds))
	for _, s := range seeds {
		if s.tenant != tid || s.amount != amount || s.currency != currency {
			continue
		}
		if l.movRepo != nil && l.movRepo.linkedDocument(s.doc.ID) {
			continue
		}
		out = append(out, s.doc)
	}
	return out, nil
}

// callCount returns the number of LinkCandidates calls.
func (l *fakeLinkLister) callCount() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.calls)
}

// --- fakePDF ----------------------------------------------------------------

// fakePDF is a configurable implementation of PDFTextExtractor.
type fakePDF struct {
	mu   sync.Mutex
	text string
	err  error
}

func (p *fakePDF) ExtractText(_ []byte) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.text, p.err
}

// --- helpers ----------------------------------------------------------------

// tenantFromOpts extracts the tenant ID from options ("" if absent).
func tenantFromOpts(opts []repo.Option) string {
	return repo.ApplyOptions(opts...).TenantID
}

// asString coerces a filter value to a string.
func asString(v any) string {
	s, _ := v.(string)
	return s
}

// deepMovement returns a deep copy of a MoneyMovement (all pointer fields).
func deepMovement(m entity.MoneyMovement) entity.MoneyMovement {
	m.SourceAccountID = copyStrPtr(m.SourceAccountID)
	m.DestinationAccountID = copyStrPtr(m.DestinationAccountID)
	m.ImportBatchID = copyStrPtr(m.ImportBatchID)
	m.ImportLine = copyIntPtr(m.ImportLine)
	m.ExternalReference = copyStrPtr(m.ExternalReference)
	m.LinkedDocumentID = copyStrPtr(m.LinkedDocumentID)
	m.LinkCreator = copyStrPtr(m.LinkCreator)
	return m
}

// deepImportLine returns a deep copy of an ImportLine (all pointer fields).
func deepImportLine(l entity.ImportLine) entity.ImportLine {
	l.OccurredOn = copyTimePtr(l.OccurredOn)
	l.Amount = copyStrPtr(l.Amount)
	l.Description = copyStrPtr(l.Description)
	l.NormDescription = copyStrPtr(l.NormDescription)
	l.ExternalReference = copyStrPtr(l.ExternalReference)
	l.ErrorReason = copyStrPtr(l.ErrorReason)
	return l
}

func copyStrPtr(p *string) *string {
	if p == nil {
		return nil
	}
	s := *p
	return &s
}

func copyIntPtr(p *int) *int {
	if p == nil {
		return nil
	}
	i := *p
	return &i
}

func copyTimePtr(p *time.Time) *time.Time {
	if p == nil {
		return nil
	}
	t := *p
	return &t
}

// --- fakeFactory --------------------------------------------------------------

// fakeFactory wraps a concrete *repo.Factory built from the fake repositories.
// InTx snapshots the movements, import-batches and import-lines repos; on
// callback error it rolls all three back.
type fakeFactory struct {
	factory     *repo.Factory
	movRepo     *fakeMovementRepo
	batchRepo   *fakeImportBatchRepo
	lineRepo    *fakeImportLineRepo
	accountRepo *fakeAccountRepo
	sourceRepo  *fakeSourceRepo
}

func newFakeFactory(sourceRepo *fakeSourceRepo, accountRepo *fakeAccountRepo, movRepo *fakeMovementRepo, batchRepo *fakeImportBatchRepo, lineRepo *fakeImportLineRepo) *fakeFactory {
	f := &repo.Factory{
		Sources:       sourceRepo,
		Accounts:      accountRepo,
		Movements:     movRepo,
		ImportBatches: batchRepo,
		ImportLines:   lineRepo,
		InTx: func(ctx context.Context, fn func(ctx context.Context, repos *repo.Repos) error) error {
			movRepo.snapshot()
			batchRepo.snapshot()
			lineRepo.snapshot()
			repos := &repo.Repos{
				Sources:       sourceRepo,
				Accounts:      accountRepo,
				Movements:     movRepo,
				ImportBatches: batchRepo,
				ImportLines:   lineRepo,
			}
			if err := fn(ctx, repos); err != nil {
				movRepo.rollback()
				batchRepo.rollback()
				lineRepo.rollback()
				return err
			}
			return nil
		},
	}
	return &fakeFactory{factory: f, movRepo: movRepo, batchRepo: batchRepo, lineRepo: lineRepo, accountRepo: accountRepo, sourceRepo: sourceRepo}
}

// --- harness ------------------------------------------------------------------

// harness wires the fakes together with the Service under test.
type harness struct {
	svc         *Service
	factory     *fakeFactory
	sourceRepo  *fakeSourceRepo
	sourceStore *fakeSourceStore
	movLister   *fakeMovLister
	linkLister  *fakeLinkLister
	pdf         *fakePDF
	maxBytes    int64
	maxLines    int
}

// newHarness builds a fully wired harness with default limits and a seeded
// account (ID "acct-1", currency INR, tenant testTenant).
func newHarness() *harness {
	sourceRepo := newFakeSourceRepo()
	accountRepo := newFakeAccountRepo()
	accountRepo.seed(entity.FinancialAccount{ID: "acct-1", TenantID: testTenant, Name: "Primary", Type: entity.AccountTypeBank, Currency: "INR"})
	movRepo := newFakeMovementRepo()
	batchRepo := newFakeImportBatchRepo()
	lineRepo := newFakeImportLineRepo()
	sourceStore := newFakeSourceStore()
	movLister := newFakeMovLister()
	linkLister := newFakeLinkLister(movRepo)
	pdf := &fakePDF{}

	maxBytes := int64(1 << 20)
	maxLines := 1000
	ff := newFakeFactory(sourceRepo, accountRepo, movRepo, batchRepo, lineRepo)
	svc := New(ff.factory, sourceStore, movLister, linkLister, pdf, maxBytes, maxLines)
	return &harness{
		svc:         svc,
		factory:     ff,
		sourceRepo:  sourceRepo,
		sourceStore: sourceStore,
		movLister:   movLister,
		linkLister:  linkLister,
		pdf:         pdf,
		maxBytes:    maxBytes,
		maxLines:    maxLines,
	}
}

// ctx returns a context carrying the given tenant ID.
func (h *harness) ctx(tenantID string) context.Context {
	return tenant.WithTenant(context.Background(), tenantID)
}

// --- Upload tests -------------------------------------------------------------

// TestUploadSuccessfulCSVPreviewBatch covers the happy path: a 3-line CSV
// (2 valid, 1 error line with a bad date) produces a preview batch with
// correct counts, retained Source, re-readable lines, and no movements.
func TestUploadSuccessfulCSVPreviewBatch(t *testing.T) {
	t.Parallel()
	h := newHarness()
	data := []byte("2026-08-20,-1250.50,Reliance Digital,REF-1\n2026-08-21,300.00,Coffee\nnot-a-date,100.00,Bad Date Line")

	batch, lines, err := h.svc.Upload(h.ctx(testTenant), "acct-1", "statement.csv", data)
	if err != nil {
		t.Fatalf("Upload returned error %v, want nil", err)
	}

	if batch.State != entity.BatchStatePreview {
		t.Fatalf("batch State = %q, want %q", batch.State, entity.BatchStatePreview)
	}
	if batch.AccountID != "acct-1" {
		t.Fatalf("batch AccountID = %q, want \"acct-1\"", batch.AccountID)
	}
	if batch.Filename != "statement.csv" {
		t.Fatalf("batch Filename = %q, want \"statement.csv\"", batch.Filename)
	}
	if batch.Format != string(FormatCSV) {
		t.Fatalf("batch Format = %q, want %q", batch.Format, string(FormatCSV))
	}
	if batch.SourceID == "" {
		t.Fatalf("batch SourceID is empty, want non-empty")
	}
	if batch.LineCountValid != 2 {
		t.Errorf("batch LineCountValid = %d, want 2", batch.LineCountValid)
	}
	if batch.LineCountDuplicate != 0 {
		t.Errorf("batch LineCountDuplicate = %d, want 0", batch.LineCountDuplicate)
	}
	if batch.LineCountPossibleDup != 0 {
		t.Errorf("batch LineCountPossibleDup = %d, want 0", batch.LineCountPossibleDup)
	}
	if batch.LineCountError != 1 {
		t.Errorf("batch LineCountError = %d, want 1", batch.LineCountError)
	}
	if batch.TenantID != testTenant {
		t.Errorf("batch TenantID = %q, want %q", batch.TenantID, testTenant)
	}

	if len(lines) != 3 {
		t.Fatalf("got %d lines, want 3", len(lines))
	}
	// Line 1: valid expense
	l1 := lines[0]
	if l1.LineRef != 1 {
		t.Errorf("line 1 LineRef = %d, want 1", l1.LineRef)
	}
	if l1.Status != entity.LineStatusValid {
		t.Errorf("line 1 Status = %q, want %q", l1.Status, entity.LineStatusValid)
	}
	if l1.OccurredOn == nil || l1.OccurredOn.Year() != 2026 {
		t.Errorf("line 1 OccurredOn = %v, want 2026-08-20", l1.OccurredOn)
	}
	if l1.Amount == nil || *l1.Amount != "1250.50" {
		t.Errorf("line 1 Amount = %v, want \"1250.50\"", l1.Amount)
	}
	if l1.Direction == nil || *l1.Direction != DirectionOut {
		t.Errorf("line 1 Direction = %v, want %q", l1.Direction, DirectionOut)
	}
	if l1.Description == nil || *l1.Description != "Reliance Digital" {
		t.Errorf("line 1 Description = %v, want \"Reliance Digital\"", l1.Description)
	}
	if l1.ExternalReference == nil || *l1.ExternalReference != "REF-1" {
		t.Errorf("line 1 ExternalReference = %v, want \"REF-1\"", l1.ExternalReference)
	}

	// Line 2: valid income
	l2 := lines[1]
	if l2.LineRef != 2 {
		t.Errorf("line 2 LineRef = %d, want 2", l2.LineRef)
	}
	if l2.Status != entity.LineStatusValid {
		t.Errorf("line 2 Status = %q, want %q", l2.Status, entity.LineStatusValid)
	}
	if l2.Direction == nil || *l2.Direction != DirectionIn {
		t.Errorf("line 2 Direction = %v, want %q", l2.Direction, DirectionIn)
	}

	// Line 3: error line
	l3 := lines[2]
	if l3.LineRef != 3 {
		t.Errorf("line 3 LineRef = %d, want 3", l3.LineRef)
	}
	if l3.Status != entity.LineStatusError {
		t.Errorf("line 3 Status = %q, want %q", l3.Status, entity.LineStatusError)
	}
	if l3.ErrorReason == nil || !strings.Contains(*l3.ErrorReason, "date") {
		t.Errorf("line 3 ErrorReason = %v, want mention of date", l3.ErrorReason)
	}

	// Source retained: store called once with original filename.
	if got := h.sourceStore.callCount(); got != 1 {
		t.Errorf("sourceStore callCount = %d, want 1", got)
	}
	name, _ := h.sourceStore.lastCall()
	if name != "statement.csv" {
		t.Errorf("sourceStore lastCall name = %q, want \"statement.csv\"", name)
	}
	// Source row persisted for the uploaded filename and tenant.
	srcs := h.sourceRepo.byFilename("statement.csv", testTenant)
	if len(srcs) != 1 {
		t.Errorf("sourceRepo row count for statement.csv/test-tenant = %d, want 1", len(srcs))
	}

	// No movements created.
	if got := h.factory.movRepo.count(); got != 0 {
		t.Errorf("movement count = %d, want 0", got)
	}

	// Re-readable via GetBatch.
	gb, glines, err := h.svc.GetBatch(h.ctx(testTenant), batch.ID)
	if err != nil {
		t.Fatalf("GetBatch returned error %v", err)
	}
	if gb.ID != batch.ID {
		t.Errorf("GetBatch batch ID = %q, want %q", gb.ID, batch.ID)
	}
	if len(glines) != 3 {
		t.Errorf("GetBatch returned %d lines, want 3", len(glines))
	}

	// fakeMovLister called with account ID and tenant.
	if got := h.movLister.callAccountID(); got != "acct-1" {
		t.Errorf("movLister accountID = %q, want \"acct-1\"", got)
	}
	if got := tenantFromOpts(h.movLister.lastCallOpts()); got != testTenant {
		t.Errorf("movLister tenant = %q, want %q", got, testTenant)
	}
}

// TestUploadUnsupportedType verifies that a store error and a non-CSV/PDF
// content type both yield ErrUnsupportedType.
func TestUploadUnsupportedType(t *testing.T) {
	t.Parallel()

	t.Run("store error", func(t *testing.T) {
		t.Parallel()
		h := newHarness()
		h.sourceStore.err = errStoreUnsupported
		_, _, err := h.svc.Upload(h.ctx(testTenant), "acct-1", "file.png", []byte("x"))
		if !errors.Is(err, ErrUnsupportedType) {
			t.Fatalf("Upload error = %v, want ErrUnsupportedType", err)
		}
		if got := h.sourceStore.callCount(); got != 1 {
			t.Errorf("sourceStore callCount = %d, want 1", got)
		}
		if got := h.factory.batchRepo.count(); got != 0 {
			t.Errorf("batch count = %d, want 0", got)
		}
	})

	t.Run("octet-stream content type", func(t *testing.T) {
		t.Parallel()
		h := newHarness()
		h.sourceStore.contentType = "application/octet-stream"
		_, _, err := h.svc.Upload(h.ctx(testTenant), "acct-1", "file.bin", []byte("2026-08-20,-100.00,Test"))
		if !errors.Is(err, ErrUnsupportedType) {
			t.Fatalf("Upload error = %v, want ErrUnsupportedType", err)
		}
		if got := h.factory.batchRepo.count(); got != 0 {
			t.Errorf("batch count = %d, want 0", got)
		}
	})
}

// TestUploadOversize verifies that oversized data is rejected before the
// store is called.
func TestUploadOversize(t *testing.T) {
	t.Parallel()
	h := newHarness()
	h.svc.maxBytes = 10
	big := bytes.Repeat([]byte("x"), 11)
	_, _, err := h.svc.Upload(h.ctx(testTenant), "acct-1", "big.csv", big)
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("Upload error = %v, want ErrTooLarge", err)
	}
	if got := h.sourceStore.callCount(); got != 0 {
		t.Errorf("sourceStore callCount = %d, want 0", got)
	}
	if got := h.factory.batchRepo.count(); got != 0 {
		t.Errorf("batch count = %d, want 0", got)
	}
	// Store never called, so no Source row either.
	if got := h.sourceRepo.count(); got != 0 {
		t.Errorf("sourceRepo count = %d, want 0 (store never called)", got)
	}
}

// TestUploadUnknownAccount verifies that a missing account propagates
// repo.ErrNotFound and the Source is retained.
func TestUploadUnknownAccount(t *testing.T) {
	t.Parallel()
	h := newHarness()
	data := []byte("2026-08-20,-100.00,Test")
	_, _, err := h.svc.Upload(h.ctx(testTenant), "nonexistent-acct", "s.csv", data)
	if !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("Upload error = %v, want repo.ErrNotFound", err)
	}
	if got := h.sourceStore.callCount(); got != 1 {
		t.Errorf("sourceStore callCount = %d, want 1 (retained)", got)
	}
	if got := h.factory.batchRepo.count(); got != 0 {
		t.Errorf("batch count = %d, want 0", got)
	}
	// Source row retained despite the unknown account.
	if srcs := h.sourceRepo.byFilename("s.csv", testTenant); len(srcs) != 1 {
		t.Errorf("sourceRepo row count for s.csv/test-tenant = %d, want 1 (retained)", len(srcs))
	}
}

// TestUploadImageOnlyPDF verifies that a PDF with no extractable text
// yields ErrNoLines and the Source is retained.
func TestUploadImageOnlyPDF(t *testing.T) {
	t.Parallel()
	h := newHarness()
	h.sourceStore.contentType = "application/pdf"
	h.pdf.text = "" // image-only PDF
	_, _, err := h.svc.Upload(h.ctx(testTenant), "acct-1", "scan.pdf", []byte("pdf-bytes"))
	if !errors.Is(err, ErrNoLines) {
		t.Fatalf("Upload error = %v, want ErrNoLines", err)
	}
	if got := h.sourceStore.callCount(); got != 1 {
		t.Errorf("sourceStore callCount = %d, want 1 (retained)", got)
	}
	if got := h.factory.batchRepo.count(); got != 0 {
		t.Errorf("batch count = %d, want 0", got)
	}
	// Source row retained despite ErrNoLines.
	if srcs := h.sourceRepo.byFilename("scan.pdf", testTenant); len(srcs) != 1 {
		t.Errorf("sourceRepo row count for scan.pdf/test-tenant = %d, want 1 (retained)", len(srcs))
	}
}

// TestUploadPDFTextLines verifies PDF text extraction and line splitting.
func TestUploadPDFTextLines(t *testing.T) {
	t.Parallel()
	h := newHarness()
	h.sourceStore.contentType = "application/pdf"
	h.pdf.text = "2026-08-20,-1250.50,Reliance Digital,REF-1\n\n2026-08-21,300.00,Coffee\n"

	batch, lines, err := h.svc.Upload(h.ctx(testTenant), "acct-1", "scan.pdf", []byte("pdf-bytes"))
	if err != nil {
		t.Fatalf("Upload returned error %v, want nil", err)
	}
	if batch.Format != string(FormatPDF) {
		t.Errorf("batch Format = %q, want %q", batch.Format, string(FormatPDF))
	}
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2", len(lines))
	}
	if lines[0].LineRef != 1 {
		t.Errorf("line 1 LineRef = %d, want 1", lines[0].LineRef)
	}
	if lines[1].LineRef != 2 {
		t.Errorf("line 2 LineRef = %d, want 2", lines[1].LineRef)
	}
	if lines[0].Status != entity.LineStatusValid {
		t.Errorf("line 1 Status = %q, want valid", lines[0].Status)
	}
	if lines[1].Status != entity.LineStatusValid {
		t.Errorf("line 2 Status = %q, want valid", lines[1].Status)
	}
}

// TestUploadTooManyLines verifies the maxLines bound.
func TestUploadTooManyLines(t *testing.T) {
	t.Parallel()
	h := newHarness()
	maxLines := 2
	h.svc.maxLines = maxLines
	// 3 data rows (no header)
	data := []byte("2026-08-20,-100.00,A\n2026-08-21,-200.00,B\n2026-08-22,-300.00,C")
	_, _, err := h.svc.Upload(h.ctx(testTenant), "acct-1", "many.csv", data)
	if !errors.Is(err, ErrTooManyLines) {
		t.Fatalf("Upload error = %v, want ErrTooManyLines", err)
	}
	if got := h.sourceStore.callCount(); got != 1 {
		t.Errorf("sourceStore callCount = %d, want 1 (retained)", got)
	}
	if got := h.factory.batchRepo.count(); got != 0 {
		t.Errorf("batch count = %d, want 0", got)
	}
	// Source row retained despite ErrTooManyLines.
	if srcs := h.sourceRepo.byFilename("many.csv", testTenant); len(srcs) != 1 {
		t.Errorf("sourceRepo row count for many.csv/test-tenant = %d, want 1 (retained)", len(srcs))
	}
}

// TestUploadInBoundsWithinTimeBound maps to the spec scenario "In-bounds file
// parses within the time bound" (specs/statement-import, requirement
// "Ingestion bounds"; design D12: the 60 s budget is verified by the 100k-line
// CSV test, not a hard runtime timeout). It uploads exactly 100,000 valid
// lines — the default max-lines bound — and asserts the Upload returns the
// preview batch within the documented 60 s budget.
func TestUploadInBoundsWithinTimeBound(t *testing.T) {
	t.Parallel()

	const wantLines = 100000 // default max-lines bound (config PROCRASTINATOR_MAX_STATEMENT_LINES)
	const budget = 60 * time.Second

	// Build exactly wantLines CSV data rows: date,amount,description with a
	// per-line description so every line has a distinct content fingerprint
	// (no within-batch duplicates) and classifies as valid.
	var buf bytes.Buffer
	for i := 1; i <= wantLines; i++ {
		fmt.Fprintf(&buf, "2026-08-01,-100.00,Statement line %d\n", i)
	}
	data := buf.Bytes()

	// The harness defaults (maxBytes 1 MiB, maxLines 1000) are below this
	// payload, so construct the service directly with the spec bounds,
	// mirroring the harness construction.
	sourceRepo := newFakeSourceRepo()
	accountRepo := newFakeAccountRepo()
	accountRepo.seed(entity.FinancialAccount{ID: "acct-1", TenantID: testTenant, Name: "Primary", Type: entity.AccountTypeBank, Currency: "INR"})
	movRepo := newFakeMovementRepo()
	batchRepo := newFakeImportBatchRepo()
	lineRepo := newFakeImportLineRepo()
	sourceStore := newFakeSourceStore()
	movLister := newFakeMovLister()
	linkLister := newFakeLinkLister(movRepo)
	ff := newFakeFactory(sourceRepo, accountRepo, movRepo, batchRepo, lineRepo)
	svc := New(ff.factory, sourceStore, movLister, linkLister, &fakePDF{}, int64(8<<20), wantLines)

	start := time.Now()
	batch, lines, err := svc.Upload(tenant.WithTenant(context.Background(), testTenant), "acct-1", "statement.csv", data)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Upload returned error %v, want nil (in-bounds file must be accepted)", err)
	}
	if batch.State != entity.BatchStatePreview {
		t.Errorf("batch State = %q, want %q", batch.State, entity.BatchStatePreview)
	}
	if batch.LineCountValid != wantLines {
		t.Errorf("batch LineCountValid = %d, want %d", batch.LineCountValid, wantLines)
	}
	if batch.LineCountDuplicate != 0 {
		t.Errorf("batch LineCountDuplicate = %d, want 0", batch.LineCountDuplicate)
	}
	if batch.LineCountPossibleDup != 0 {
		t.Errorf("batch LineCountPossibleDup = %d, want 0", batch.LineCountPossibleDup)
	}
	if batch.LineCountError != 0 {
		t.Errorf("batch LineCountError = %d, want 0", batch.LineCountError)
	}
	if len(lines) != wantLines {
		t.Fatalf("got %d lines, want %d", len(lines), wantLines)
	}
	for i, l := range lines {
		if l.Status != entity.LineStatusValid {
			t.Fatalf("line %d Status = %q, want %q", i+1, l.Status, entity.LineStatusValid)
		}
	}
	if elapsed >= budget {
		t.Errorf("Upload took %v, want strictly less than %v (documented ingestion budget)", elapsed, budget)
	}
}

// TestUploadNoTenant verifies that a context without a tenant is rejected
// before any store call.
func TestUploadNoTenant(t *testing.T) {
	t.Parallel()
	h := newHarness()
	_, _, err := h.svc.Upload(context.Background(), "acct-1", "s.csv", []byte("2026-08-20,-100.00,Test"))
	if !errors.Is(err, tenant.ErrNoTenant) {
		t.Fatalf("Upload error = %v, want tenant.ErrNoTenant", err)
	}
	if got := h.sourceStore.callCount(); got != 0 {
		t.Errorf("sourceStore callCount = %d, want 0", got)
	}
	if got := h.factory.batchRepo.count(); got != 0 {
		t.Errorf("batch count = %d, want 0", got)
	}
}

// --- Commit tests -------------------------------------------------------------

// seedPreviewBatch seeds a preview batch with the given lines (bypassing
// Upload) and returns the batch ID. Lines must already have BatchID set to
// the batch ID (set after seeding the batch).
func seedPreviewBatch(h *harness, batchID string, lines []entity.ImportLine) entity.ImportBatch {
	batch := entity.ImportBatch{
		ID:        batchID,
		TenantID:  testTenant,
		State:     entity.BatchStatePreview,
		AccountID: "acct-1",
		SourceID:  "src-seeded",
		Filename:  "seeded.csv",
		Format:    "csv",
		CreatedAt: fixedT0,
		UpdatedAt: fixedT0,
	}
	h.factory.batchRepo.seed(batch)
	for _, l := range lines {
		l.BatchID = batchID
		l.TenantID = testTenant
		h.factory.lineRepo.seed(l)
	}
	// Recompute counts on the batch.
	counts := computeCounts(lines)
	batch.LineCountValid = counts.valid
	batch.LineCountDuplicate = counts.dup
	batch.LineCountPossibleDup = counts.possible
	batch.LineCountError = counts.errCount
	h.factory.batchRepo.seed(batch)
	return batch
}

type statusCounts struct {
	valid, dup, possible, errCount int
}

func computeCounts(lines []entity.ImportLine) statusCounts {
	var c statusCounts
	for _, l := range lines {
		switch l.Status {
		case entity.LineStatusValid:
			c.valid++
		case entity.LineStatusDuplicate:
			c.dup++
		case entity.LineStatusPossibleDuplicate:
			c.possible++
		case entity.LineStatusError:
			c.errCount++
		}
	}
	return c
}

// TestCommitCreatesMovementsForValidLinesOnly verifies that only valid lines
// produce movements, with correct provenance.
func TestCommitCreatesMovementsForValidLinesOnly(t *testing.T) {
	t.Parallel()
	h := newHarness()

	day0 := fixedDay
	day1 := fixedDay1
	amount1 := "1250.50"
	amount2 := "300.00"
	amount3 := "999.99"
	desc1 := "Reliance Digital"
	desc2 := "Coffee"
	desc3 := "Unknown"
	ref1 := "REF-1"
	ref3 := "REF-3"
	norm1 := "reliance digital"
	norm2 := "coffee"
	norm3 := "unknown"

	lines := []entity.ImportLine{
		{
			ID: "line-1", LineRef: 1, RawLine: "2026-08-20,-1250.50,Reliance Digital,REF-1",
			OccurredOn: &day0, Amount: &amount1, Direction: ptr(DirectionOut),
			Description: &desc1, NormDescription: &norm1, ExternalReference: &ref1,
			Status: entity.LineStatusValid,
		},
		{
			ID: "line-2", LineRef: 2, RawLine: "2026-08-21,300.00,Coffee",
			OccurredOn: &day1, Amount: &amount2, Direction: ptr(DirectionIn),
			Description: &desc2, NormDescription: &norm2,
			Status: entity.LineStatusValid,
		},
		{
			ID: "line-3", LineRef: 3, RawLine: "2026-08-20,999.99,Unknown,REF-3",
			OccurredOn: &day0, Amount: &amount3, Direction: ptr(DirectionIn),
			Description: &desc3, NormDescription: &norm3, ExternalReference: &ref3,
			Status: entity.LineStatusDuplicate,
		},
		{
			ID: "line-4", LineRef: 4, RawLine: "bad-line",
			Status: entity.LineStatusError, ErrorReason: ptr("malformed"),
		},
	}

	batch := seedPreviewBatch(h, "batch-1", lines)
	summary, err := h.svc.Commit(h.ctx(testTenant), batch.ID)
	if err != nil {
		t.Fatalf("Commit returned error %v, want nil", err)
	}

	if summary.Created != 2 {
		t.Errorf("summary.Created = %d, want 2", summary.Created)
	}
	if summary.Skipped != 2 {
		t.Errorf("summary.Skipped = %d, want 2 (duplicate + error)", summary.Skipped)
	}

	movs := h.factory.movRepo.all()
	if len(movs) != 2 {
		t.Fatalf("movement count = %d, want 2", len(movs))
	}

	// Verify movement 1 (expense from line 1).
	mv1 := movs[0] // ID order: mv-1, mv-2
	if mv1.Kind != entity.KindExpense {
		t.Errorf("mv1 Kind = %q, want %q", mv1.Kind, entity.KindExpense)
	}
	if mv1.Amount != "1250.50" {
		t.Errorf("mv1 Amount = %q, want \"1250.50\"", mv1.Amount)
	}
	if mv1.Currency != "INR" {
		t.Errorf("mv1 Currency = %q, want \"INR\"", mv1.Currency)
	}
	if mv1.SourceAccountID == nil || *mv1.SourceAccountID != "acct-1" {
		t.Errorf("mv1 SourceAccountID = %v, want &\"acct-1\"", mv1.SourceAccountID)
	}
	if mv1.DestinationAccountID != nil {
		t.Errorf("mv1 DestinationAccountID = %v, want nil", mv1.DestinationAccountID)
	}
	if mv1.Origin != entity.OriginImport {
		t.Errorf("mv1 Origin = %q, want %q", mv1.Origin, entity.OriginImport)
	}
	if mv1.ImportBatchID == nil || *mv1.ImportBatchID != "batch-1" {
		t.Errorf("mv1 ImportBatchID = %v, want &\"batch-1\"", mv1.ImportBatchID)
	}
	if mv1.ImportLine == nil || *mv1.ImportLine != 1 {
		t.Errorf("mv1 ImportLine = %v, want &1", mv1.ImportLine)
	}
	if mv1.ExternalReference == nil || *mv1.ExternalReference != "REF-1" {
		t.Errorf("mv1 ExternalReference = %v, want &\"REF-1\"", mv1.ExternalReference)
	}
	if mv1.Description != "Reliance Digital" {
		t.Errorf("mv1 Description = %q, want \"Reliance Digital\"", mv1.Description)
	}

	// Verify movement 2 (income from line 2).
	mv2 := movs[1]
	if mv2.Kind != entity.KindIncome {
		t.Errorf("mv2 Kind = %q, want %q", mv2.Kind, entity.KindIncome)
	}
	if mv2.DestinationAccountID == nil || *mv2.DestinationAccountID != "acct-1" {
		t.Errorf("mv2 DestinationAccountID = %v, want &\"acct-1\"", mv2.DestinationAccountID)
	}
	if mv2.SourceAccountID != nil {
		t.Errorf("mv2 SourceAccountID = %v, want nil", mv2.SourceAccountID)
	}

	// Batch is committed.
	gb, _, err := h.svc.GetBatch(h.ctx(testTenant), "batch-1")
	if err != nil {
		t.Fatalf("GetBatch error %v", err)
	}
	if gb.State != entity.BatchStateCommitted {
		t.Errorf("batch State = %q, want %q", gb.State, entity.BatchStateCommitted)
	}
	if gb.LineCountValid != 2 {
		t.Errorf("batch LineCountValid = %d, want 2", gb.LineCountValid)
	}
}

// TestCommitAtomic verifies that a mid-tx failure rolls back all movements
// and leaves the batch in preview state.
func TestCommitAtomic(t *testing.T) {
	t.Parallel()
	h := newHarness()

	day0 := fixedDay
	amount1 := "100.00"
	amount2 := "200.00"
	desc1 := "First"
	desc2 := "Second"
	norm1 := "first"
	norm2 := "second"

	lines := []entity.ImportLine{
		{
			ID: "line-1", LineRef: 1, RawLine: "l1",
			OccurredOn: &day0, Amount: &amount1, Direction: ptr(DirectionOut),
			Description: &desc1, NormDescription: &norm1,
			Status: entity.LineStatusValid,
		},
		{
			ID: "line-2", LineRef: 2, RawLine: "l2",
			OccurredOn: &day0, Amount: &amount2, Direction: ptr(DirectionIn),
			Description: &desc2, NormDescription: &norm2,
			Status: entity.LineStatusValid,
		},
	}

	batch := seedPreviewBatch(h, "batch-1", lines)

	// Inject failure on the 2nd movement Create.
	h.factory.movRepo.failAt = 2

	_, err := h.svc.Commit(h.ctx(testTenant), batch.ID)
	if err == nil {
		t.Fatal("Commit returned nil error, want error from failed create")
	}
	if !errors.Is(err, errMoveCreateFail) {
		t.Errorf("Commit error = %v, want errMoveCreateFail", err)
	}

	// Zero movements after rollback.
	if got := h.factory.movRepo.count(); got != 0 {
		t.Errorf("movement count = %d, want 0 after rollback", got)
	}

	// Batch still in preview.
	gb, _, err := h.svc.GetBatch(h.ctx(testTenant), "batch-1")
	if err != nil {
		t.Fatalf("GetBatch error %v", err)
	}
	if gb.State != entity.BatchStatePreview {
		t.Errorf("batch State = %q, want %q (preview after rollback)", gb.State, entity.BatchStatePreview)
	}
}

// TestCommitIdempotent verifies that committing an already-committed batch
// returns the same summary without creating new movements.
func TestCommitIdempotent(t *testing.T) {
	t.Parallel()
	h := newHarness()

	day0 := fixedDay
	amount1 := "100.00"
	desc1 := "First"
	norm1 := "first"

	lines := []entity.ImportLine{
		{
			ID: "line-1", LineRef: 1, RawLine: "l1",
			OccurredOn: &day0, Amount: &amount1, Direction: ptr(DirectionOut),
			Description: &desc1, NormDescription: &norm1,
			Status: entity.LineStatusValid,
		},
		{
			ID: "line-2", LineRef: 2, RawLine: "l2",
			Status: entity.LineStatusError, ErrorReason: ptr("bad"),
		},
	}

	batch := seedPreviewBatch(h, "batch-1", lines)

	summary1, err := h.svc.Commit(h.ctx(testTenant), batch.ID)
	if err != nil {
		t.Fatalf("first Commit error %v", err)
	}
	movCountAfterFirst := h.factory.movRepo.count()

	summary2, err := h.svc.Commit(h.ctx(testTenant), batch.ID)
	if err != nil {
		t.Fatalf("second Commit error %v", err)
	}

	if summary1.Created != summary2.Created {
		t.Errorf("summary.Created: first=%d second=%d, want equal", summary1.Created, summary2.Created)
	}
	if summary1.Skipped != summary2.Skipped {
		t.Errorf("summary.Skipped: first=%d second=%d, want equal", summary1.Skipped, summary2.Skipped)
	}
	if h.factory.movRepo.count() != movCountAfterFirst {
		t.Errorf("movement count changed after re-commit: was %d now %d", movCountAfterFirst, h.factory.movRepo.count())
	}
}

// TestCommitDiscardedConflict verifies that committing a discarded batch
// returns ErrConflict.
func TestCommitDiscardedConflict(t *testing.T) {
	t.Parallel()
	h := newHarness()

	batch := entity.ImportBatch{
		ID: "batch-1", TenantID: testTenant,
		State: entity.BatchStateDiscarded, AccountID: "acct-1",
		CreatedAt: fixedT0, UpdatedAt: fixedT0,
	}
	h.factory.batchRepo.seed(batch)

	_, err := h.svc.Commit(h.ctx(testTenant), "batch-1")
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("Commit error = %v, want ErrConflict", err)
	}
	if got := h.factory.movRepo.count(); got != 0 {
		t.Errorf("movement count = %d, want 0", got)
	}
}

// TestCommitZeroValidLines verifies that a batch with only duplicate/error
// lines commits with 0 movements.
func TestCommitZeroValidLines(t *testing.T) {
	t.Parallel()
	h := newHarness()

	lines := []entity.ImportLine{
		{ID: "line-1", LineRef: 1, RawLine: "l1", Status: entity.LineStatusDuplicate},
		{ID: "line-2", LineRef: 2, RawLine: "l2", Status: entity.LineStatusError, ErrorReason: ptr("bad")},
	}

	seedPreviewBatch(h, "batch-1", lines)
	summary, err := h.svc.Commit(h.ctx(testTenant), "batch-1")
	if err != nil {
		t.Fatalf("Commit error %v", err)
	}
	if summary.Created != 0 {
		t.Errorf("summary.Created = %d, want 0", summary.Created)
	}
	if summary.Skipped != 2 {
		t.Errorf("summary.Skipped = %d, want 2", summary.Skipped)
	}
	if got := h.factory.movRepo.count(); got != 0 {
		t.Errorf("movement count = %d, want 0", got)
	}
}

// TestCommitUnknownBatch verifies that committing a non-existent batch
// returns repo.ErrNotFound.
func TestCommitUnknownBatch(t *testing.T) {
	t.Parallel()
	h := newHarness()
	_, err := h.svc.Commit(h.ctx(testTenant), "nonexistent")
	if !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("Commit error = %v, want repo.ErrNotFound", err)
	}
}

// TestCommitNoTenant verifies that a context without a tenant is rejected.
func TestCommitNoTenant(t *testing.T) {
	t.Parallel()
	h := newHarness()
	lines := []entity.ImportLine{
		{ID: "line-1", LineRef: 1, RawLine: "l1", Status: entity.LineStatusValid,
			OccurredOn: ptr(fixedDay), Amount: ptr("100.00"), Direction: ptr(DirectionOut),
			Description: ptr("x"), NormDescription: ptr("x")},
	}
	seedPreviewBatch(h, "batch-1", lines)

	_, err := h.svc.Commit(context.Background(), "batch-1")
	if !errors.Is(err, tenant.ErrNoTenant) {
		t.Fatalf("Commit error = %v, want tenant.ErrNoTenant", err)
	}
	if got := h.factory.movRepo.count(); got != 0 {
		t.Errorf("movement count = %d, want 0", got)
	}
}

// --- Discard tests ------------------------------------------------------------

// TestDiscardPreview verifies that discarding a preview batch transitions it
// to discarded and lines remain readable.
func TestDiscardPreview(t *testing.T) {
	t.Parallel()
	h := newHarness()

	lines := []entity.ImportLine{
		{ID: "line-1", LineRef: 1, RawLine: "l1", Status: entity.LineStatusValid},
	}
	seedPreviewBatch(h, "batch-1", lines)

	updated, err := h.svc.Discard(h.ctx(testTenant), "batch-1")
	if err != nil {
		t.Fatalf("Discard error %v", err)
	}
	if updated.State != entity.BatchStateDiscarded {
		t.Errorf("batch State = %q, want %q", updated.State, entity.BatchStateDiscarded)
	}

	// Lines still readable.
	_, glines, err := h.svc.GetBatch(h.ctx(testTenant), "batch-1")
	if err != nil {
		t.Fatalf("GetBatch error %v", err)
	}
	if len(glines) != 1 {
		t.Errorf("GetBatch returned %d lines, want 1", len(glines))
	}
}

// TestDiscardIdempotent verifies that discarding an already-discarded batch
// returns the batch unchanged.
func TestDiscardIdempotent(t *testing.T) {
	t.Parallel()
	h := newHarness()

	batch := entity.ImportBatch{
		ID: "batch-1", TenantID: testTenant,
		State: entity.BatchStateDiscarded, AccountID: "acct-1",
		CreatedAt: fixedT0, UpdatedAt: fixedT0,
	}
	h.factory.batchRepo.seed(batch)

	updated, err := h.svc.Discard(h.ctx(testTenant), "batch-1")
	if err != nil {
		t.Fatalf("Discard error %v", err)
	}
	if updated.State != entity.BatchStateDiscarded {
		t.Errorf("batch State = %q, want %q", updated.State, entity.BatchStateDiscarded)
	}
}

// TestDiscardCommittedConflict verifies that discarding a committed batch
// returns ErrConflict.
func TestDiscardCommittedConflict(t *testing.T) {
	t.Parallel()
	h := newHarness()

	batch := entity.ImportBatch{
		ID: "batch-1", TenantID: testTenant,
		State: entity.BatchStateCommitted, AccountID: "acct-1",
		CreatedAt: fixedT0, UpdatedAt: fixedT0,
	}
	h.factory.batchRepo.seed(batch)

	_, err := h.svc.Discard(h.ctx(testTenant), "batch-1")
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("Discard error = %v, want ErrConflict", err)
	}
}

// TestDiscardUnknown verifies that discarding a non-existent batch returns
// repo.ErrNotFound.
func TestDiscardUnknown(t *testing.T) {
	t.Parallel()
	h := newHarness()
	_, err := h.svc.Discard(h.ctx(testTenant), "nonexistent")
	if !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("Discard error = %v, want repo.ErrNotFound", err)
	}
}

// --- Auto-link tests -----------------------------------------------------------

// seedCommittedBatchWithMovs seeds a committed batch and its movements
// directly (bypassing Commit) for auto-link tests.
func seedCommittedBatchWithMovs(h *harness, batchID string, batch entity.ImportBatch, movs []entity.MoneyMovement) {
	for i := range movs {
		movs[i].ImportBatchID = ptr(batchID)
		movs[i].Origin = entity.OriginImport
		movs[i].TenantID = testTenant
	}
	for i, m := range movs {
		m.CreatedAt = fixedT0.Add(time.Duration(i) * time.Second)
		h.factory.movRepo.seed(m)
	}
	batch.State = entity.BatchStateCommitted
	batch.TenantID = testTenant
	if batch.CreatedAt.IsZero() {
		batch.CreatedAt = fixedT0
	}
	if batch.UpdatedAt.IsZero() {
		batch.UpdatedAt = fixedT0
	}
	h.factory.batchRepo.seed(batch)
}

// TestAutoLinkExactlyOne verifies that a single matching unlinked document
// is linked to the movement with LinkCreator "auto".
func TestAutoLinkExactlyOne(t *testing.T) {
	t.Parallel()
	h := newHarness()

	amount := "500.00"
	currency := "INR"
	mv := entity.MoneyMovement{
		ID: "mv-1", Kind: entity.KindExpense, Amount: amount, Currency: currency,
		SourceAccountID: ptr("acct-1"),
	}
	doc := entity.Document{ID: "doc-1", TenantID: testTenant}
	h.linkLister.seed(doc, amount, currency, testTenant)

	batch := entity.ImportBatch{ID: "batch-1", AccountID: "acct-1", LineCountValid: 1}
	seedCommittedBatchWithMovs(h, "batch-1", batch, []entity.MoneyMovement{mv})

	// Call applyAutoLinks via Commit (idempotent path).
	_, err := h.svc.Commit(h.ctx(testTenant), "batch-1")
	if err != nil {
		t.Fatalf("Commit error %v", err)
	}

	stored, _ := h.factory.movRepo.Get(context.Background(), "mv-1", repo.Tenant(testTenant))
	if stored.LinkedDocumentID == nil || *stored.LinkedDocumentID != "doc-1" {
		t.Errorf("LinkedDocumentID = %v, want &\"doc-1\"", stored.LinkedDocumentID)
	}
	if stored.LinkCreator == nil || *stored.LinkCreator != entity.LinkCreatorAuto {
		t.Errorf("LinkCreator = %v, want %q", stored.LinkCreator, entity.LinkCreatorAuto)
	}
}

// TestAutoLinkMultipleNoLink verifies that multiple matching documents result
// in no link (ambiguous).
func TestAutoLinkMultipleNoLink(t *testing.T) {
	t.Parallel()
	h := newHarness()

	amount := "500.00"
	currency := "INR"
	mv := entity.MoneyMovement{
		ID: "mv-1", Kind: entity.KindExpense, Amount: amount, Currency: currency,
		SourceAccountID: ptr("acct-1"),
	}
	h.linkLister.seed(entity.Document{ID: "doc-1", TenantID: testTenant}, amount, currency, testTenant)
	h.linkLister.seed(entity.Document{ID: "doc-2", TenantID: testTenant}, amount, currency, testTenant)

	batch := entity.ImportBatch{ID: "batch-1", AccountID: "acct-1", LineCountValid: 1}
	seedCommittedBatchWithMovs(h, "batch-1", batch, []entity.MoneyMovement{mv})

	_, err := h.svc.Commit(h.ctx(testTenant), "batch-1")
	if err != nil {
		t.Fatalf("Commit error %v", err)
	}

	stored, _ := h.factory.movRepo.Get(context.Background(), "mv-1", repo.Tenant(testTenant))
	if stored.LinkedDocumentID != nil {
		t.Errorf("LinkedDocumentID = %v, want nil (ambiguous)", stored.LinkedDocumentID)
	}
}

// TestAutoLinkZeroNoLink verifies that no matching documents result in no link.
func TestAutoLinkZeroNoLink(t *testing.T) {
	t.Parallel()
	h := newHarness()

	amount := "500.00"
	mv := entity.MoneyMovement{
		ID: "mv-1", Kind: entity.KindExpense, Amount: amount, Currency: "INR",
		SourceAccountID: ptr("acct-1"),
	}
	// No documents seeded.
	batch := entity.ImportBatch{ID: "batch-1", AccountID: "acct-1", LineCountValid: 1}
	seedCommittedBatchWithMovs(h, "batch-1", batch, []entity.MoneyMovement{mv})

	_, err := h.svc.Commit(h.ctx(testTenant), "batch-1")
	if err != nil {
		t.Fatalf("Commit error %v", err)
	}

	stored, _ := h.factory.movRepo.Get(context.Background(), "mv-1", repo.Tenant(testTenant))
	if stored.LinkedDocumentID != nil {
		t.Errorf("LinkedDocumentID = %v, want nil (no candidates)", stored.LinkedDocumentID)
	}
}

// TestAutoLinkAlreadyLinkedNotCandidate verifies that once a document is
// linked to one movement, it is no longer a candidate for another.
func TestAutoLinkAlreadyLinkedNotCandidate(t *testing.T) {
	t.Parallel()
	h := newHarness()

	amount := "500.00"
	currency := "INR"
	mv1 := entity.MoneyMovement{
		ID: "mv-1", Kind: entity.KindExpense, Amount: amount, Currency: currency,
		SourceAccountID: ptr("acct-1"),
	}
	mv2 := entity.MoneyMovement{
		ID: "mv-2", Kind: entity.KindExpense, Amount: amount, Currency: currency,
		SourceAccountID: ptr("acct-1"),
	}
	doc := entity.Document{ID: "doc-1", TenantID: testTenant}
	h.linkLister.seed(doc, amount, currency, testTenant)

	batch := entity.ImportBatch{ID: "batch-1", AccountID: "acct-1", LineCountValid: 2}
	seedCommittedBatchWithMovs(h, "batch-1", batch, []entity.MoneyMovement{mv1, mv2})

	_, err := h.svc.Commit(h.ctx(testTenant), "batch-1")
	if err != nil {
		t.Fatalf("Commit error %v", err)
	}

	mv1Stored, _ := h.factory.movRepo.Get(context.Background(), "mv-1", repo.Tenant(testTenant))
	mv2Stored, _ := h.factory.movRepo.Get(context.Background(), "mv-2", repo.Tenant(testTenant))

	// Exactly one movement should be linked.
	if mv1Stored.LinkedDocumentID != nil && mv2Stored.LinkedDocumentID != nil {
		t.Error("both movements linked, want at most one")
	}
	if mv1Stored.LinkedDocumentID == nil && mv2Stored.LinkedDocumentID == nil {
		t.Error("no movement linked, want exactly one")
	}

	// Re-commit: still exactly one link.
	_, err = h.svc.Commit(h.ctx(testTenant), "batch-1")
	if err != nil {
		t.Fatalf("re-Commit error %v", err)
	}
	mv1Stored, _ = h.factory.movRepo.Get(context.Background(), "mv-1", repo.Tenant(testTenant))
	mv2Stored, _ = h.factory.movRepo.Get(context.Background(), "mv-2", repo.Tenant(testTenant))
	links := 0
	if mv1Stored.LinkedDocumentID != nil {
		links++
	}
	if mv2Stored.LinkedDocumentID != nil {
		links++
	}
	if links != 1 {
		t.Errorf("link count after re-commit = %d, want 1", links)
	}
}

// --- Read/list tests -----------------------------------------------------------

// TestListBatchesEmpty verifies that ListBatches returns a non-nil empty slice.
func TestListBatchesEmpty(t *testing.T) {
	t.Parallel()
	h := newHarness()
	batches, err := h.svc.ListBatches(h.ctx(testTenant))
	if err != nil {
		t.Fatalf("ListBatches error %v", err)
	}
	if batches == nil {
		t.Fatal("ListBatches returned nil, want non-nil empty slice")
	}
	if len(batches) != 0 {
		t.Errorf("len(batches) = %d, want 0", len(batches))
	}
}

// TestListBatchesOrder verifies the ordering: by CreatedAt then by ID.
func TestListBatchesOrder(t *testing.T) {
	t.Parallel()
	h := newHarness()

	// Two batches with the same CreatedAt.
	h.factory.batchRepo.seed(entity.ImportBatch{
		ID: "batch-b", TenantID: testTenant, State: entity.BatchStatePreview,
		CreatedAt: fixedT1, UpdatedAt: fixedT1,
	})
	h.factory.batchRepo.seed(entity.ImportBatch{
		ID: "batch-a", TenantID: testTenant, State: entity.BatchStatePreview,
		CreatedAt: fixedT1, UpdatedAt: fixedT1,
	})
	// One batch with an earlier CreatedAt.
	h.factory.batchRepo.seed(entity.ImportBatch{
		ID: "batch-early", TenantID: testTenant, State: entity.BatchStatePreview,
		CreatedAt: fixedT0, UpdatedAt: fixedT0,
	})

	batches, err := h.svc.ListBatches(h.ctx(testTenant))
	if err != nil {
		t.Fatalf("ListBatches error %v", err)
	}
	if len(batches) != 3 {
		t.Fatalf("len(batches) = %d, want 3", len(batches))
	}
	// Order: batch-early (fixedT0), batch-a (fixedT1, "a" < "b"), batch-b (fixedT1).
	if batches[0].ID != "batch-early" {
		t.Errorf("batches[0].ID = %q, want \"batch-early\"", batches[0].ID)
	}
	if batches[1].ID != "batch-a" {
		t.Errorf("batches[1].ID = %q, want \"batch-a\" (tie broken by ID)", batches[1].ID)
	}
	if batches[2].ID != "batch-b" {
		t.Errorf("batches[2].ID = %q, want \"batch-b\"", batches[2].ID)
	}
}

// TestGetBatchUnknown verifies that GetBatch on a non-existent batch returns
// repo.ErrNotFound.
func TestGetBatchUnknown(t *testing.T) {
	t.Parallel()
	h := newHarness()
	_, _, err := h.svc.GetBatch(h.ctx(testTenant), "nonexistent")
	if !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("GetBatch error = %v, want repo.ErrNotFound", err)
	}
}

// TestGetBatchReturnsStateCountsLines verifies that GetBatch returns the
// batch with state, counts, and all lines after upload + commit.
func TestGetBatchReturnsStateCountsLines(t *testing.T) {
	t.Parallel()
	h := newHarness()

	day0 := fixedDay
	amount1 := "100.00"
	desc1 := "Test"
	norm1 := "test"

	lines := []entity.ImportLine{
		{
			ID: "line-1", LineRef: 1, RawLine: "l1",
			OccurredOn: &day0, Amount: &amount1, Direction: ptr(DirectionOut),
			Description: &desc1, NormDescription: &norm1,
			Status: entity.LineStatusValid,
		},
		{
			ID: "line-2", LineRef: 2, RawLine: "l2",
			Status: entity.LineStatusDuplicate,
		},
	}
	batch := seedPreviewBatch(h, "batch-1", lines)

	_, err := h.svc.Commit(h.ctx(testTenant), batch.ID)
	if err != nil {
		t.Fatalf("Commit error %v", err)
	}

	gb, glines, err := h.svc.GetBatch(h.ctx(testTenant), "batch-1")
	if err != nil {
		t.Fatalf("GetBatch error %v", err)
	}
	if gb.State != entity.BatchStateCommitted {
		t.Errorf("State = %q, want %q", gb.State, entity.BatchStateCommitted)
	}
	if gb.LineCountValid != 1 {
		t.Errorf("LineCountValid = %d, want 1", gb.LineCountValid)
	}
	if gb.LineCountDuplicate != 1 {
		t.Errorf("LineCountDuplicate = %d, want 1", gb.LineCountDuplicate)
	}
	if len(glines) != 2 {
		t.Errorf("len(glines) = %d, want 2", len(glines))
	}
	if glines[0].Status != entity.LineStatusValid {
		t.Errorf("glines[0].Status = %q, want valid", glines[0].Status)
	}
	if glines[1].Status != entity.LineStatusDuplicate {
		t.Errorf("glines[1].Status = %q, want duplicate", glines[1].Status)
	}
	if gb.SourceID == "" {
		t.Error("SourceID is empty, want non-empty")
	}
}

// --- Tenant tests ---------------------------------------------------------------

// TestTenantForeignBatchNotFound verifies that a batch created under one
// tenant is not visible from another tenant.
func TestTenantForeignBatchNotFound(t *testing.T) {
	t.Parallel()
	h := newHarness()

	lines := []entity.ImportLine{
		{ID: "line-1", LineRef: 1, RawLine: "l1", Status: entity.LineStatusValid},
	}
	batch := seedPreviewBatch(h, "batch-1", lines)

	// GetBatch with testTenantB.
	_, _, err := h.svc.GetBatch(h.ctx(testTenantB), batch.ID)
	if !errors.Is(err, repo.ErrNotFound) {
		t.Errorf("GetBatch (tenant B) error = %v, want repo.ErrNotFound", err)
	}

	// Commit with testTenantB.
	_, err = h.svc.Commit(h.ctx(testTenantB), batch.ID)
	if !errors.Is(err, repo.ErrNotFound) {
		t.Errorf("Commit (tenant B) error = %v, want repo.ErrNotFound", err)
	}

	// Discard with testTenantB.
	_, err = h.svc.Discard(h.ctx(testTenantB), batch.ID)
	if !errors.Is(err, repo.ErrNotFound) {
		t.Errorf("Discard (tenant B) error = %v, want repo.ErrNotFound", err)
	}
}

// TestDuplicateDetectionIgnoresOtherTenants verifies that duplicate detection
// does not consider movements from other tenants.
func TestDuplicateDetectionIgnoresOtherTenants(t *testing.T) {
	t.Parallel()
	h := newHarness()

	// testTenantB has a committed movement with ext ref TXN-9.
	extRef := "TXN-9"
	otherAcct := "acct-other"
	h.movLister.seed(entity.MoneyMovement{
		ID:                "mv-foreign",
		TenantID:          testTenantB,
		ExternalReference: &extRef,
		SourceAccountID:   &otherAcct,
	})

	// testTenant uploads a line with ext ref TXN-9 for its own account.
	data := []byte("2026-08-20,-100.00,Test,TXN-9")
	batch, lines, err := h.svc.Upload(h.ctx(testTenant), "acct-1", "s.csv", data)
	if err != nil {
		t.Fatalf("Upload error %v", err)
	}

	// The line should be valid (not duplicate) because the foreign movement
	// is in a different tenant.
	if batch.LineCountValid != 1 {
		t.Errorf("LineCountValid = %d, want 1", batch.LineCountValid)
	}
	if batch.LineCountDuplicate != 0 {
		t.Errorf("LineCountDuplicate = %d, want 0", batch.LineCountDuplicate)
	}
	if len(lines) != 1 || lines[0].Status != entity.LineStatusValid {
		t.Errorf("lines = %v, want 1 valid line", lines)
	}

	// MovementsForAccount was called with repo.Tenant(testTenant).
	if got := tenantFromOpts(h.movLister.lastCallOpts()); got != testTenant {
		t.Errorf("movLister tenant = %q, want %q", got, testTenant)
	}
}

// TestListBatchesNoTenant verifies that ListBatches without a tenant returns
// tenant.ErrNoTenant.
func TestListBatchesNoTenant(t *testing.T) {
	t.Parallel()
	h := newHarness()
	_, err := h.svc.ListBatches(context.Background())
	if !errors.Is(err, tenant.ErrNoTenant) {
		t.Fatalf("ListBatches error = %v, want tenant.ErrNoTenant", err)
	}
}

// TestGetBatchNoTenant verifies that GetBatch without a tenant returns
// tenant.ErrNoTenant.
func TestGetBatchNoTenant(t *testing.T) {
	t.Parallel()
	h := newHarness()
	_, _, err := h.svc.GetBatch(context.Background(), "batch-1")
	if !errors.Is(err, tenant.ErrNoTenant) {
		t.Fatalf("GetBatch error = %v, want tenant.ErrNoTenant", err)
	}
}

// ptr is a helper to take the address of a literal.
func ptr[T any](v T) *T {
	return &v
}
