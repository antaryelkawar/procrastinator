package ledger

import (
	"context"
	"errors"
	"math/big"
	"reflect"
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

// testTenant / testTenantB are the two tenants used across ledger tests.
const (
	testTenant  = "test-tenant"
	testTenantB = "test-tenant-b"
)

// testBase is the deterministic creation timestamp base. The nth created row
// gets base + n seconds, so creation order == (created_at, id) order.
var testBase = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func ctxWithTenant(id string) context.Context {
	return tenant.WithTenant(context.Background(), id)
}

func strPtr(s string) *string {
	return &s
}

// ---------------------------------------------------------------------------
// shared environment
// ---------------------------------------------------------------------------

// testEnv wires fresh fakes + balancer + the service under test.
type testEnv struct {
	svc       *Service
	accounts  *fakeAccountRepo
	movements *fakeMovementRepo
	docs      *fakeDocumentRepo
	calls     *callCounter
}

// newTestEnv builds a fresh environment.
func newTestEnv() *testEnv {
	calls := &callCounter{mu: &sync.Mutex{}}
	accounts := &fakeAccountRepo{rows: make(map[string]entity.FinancialAccount), calls: calls}
	movements := &fakeMovementRepo{rows: make(map[string]entity.MoneyMovement), calls: calls}
	docs := &fakeDocumentRepo{rows: make(map[string]entity.Document), calls: calls}
	factory := &repo.Factory{
		Accounts:  accounts,
		Movements: movements,
		Documents: docs,
	}
	balancer := &fakeBalancer{movements: movements}
	svc := New(factory, balancer)
	return &testEnv{svc: svc, accounts: accounts, movements: movements, docs: docs, calls: calls}
}

// totalCalls returns the cumulative repository call count across all fakes.
func (e *testEnv) totalCalls() int {
	e.calls.mu.Lock()
	defer e.calls.mu.Unlock()
	return e.calls.n
}

// callCounter counts every public repository method invocation.
type callCounter struct {
	mu *sync.Mutex
	n  int
}

func (c *callCounter) bump() {
	c.mu.Lock()
	c.n++
	c.mu.Unlock()
}

// ---------------------------------------------------------------------------
// fake repositories
// ---------------------------------------------------------------------------

// Compile-time interface guards.
var (
	_ repo.Repository[entity.FinancialAccount] = (*fakeAccountRepo)(nil)
	_ repo.Repository[entity.MoneyMovement]    = (*fakeMovementRepo)(nil)
	_ repo.Repository[entity.Document]         = (*fakeDocumentRepo)(nil)
	_ BalanceQuerier                           = (*fakeBalancer)(nil)
)

// fakeAccountRepo is an in-memory implementation of
// repo.Repository[entity.FinancialAccount] with shared call counting.
type fakeAccountRepo struct {
	mu     sync.Mutex
	rows   map[string]entity.FinancialAccount
	nextID int
	calls  *callCounter
}

func (r *fakeAccountRepo) Get(ctx context.Context, id string, opts ...repo.Option) (entity.FinancialAccount, error) {
	r.calls.bump()
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.rows[id]
	if !ok {
		return entity.FinancialAccount{}, repo.ErrNotFound
	}
	if tid := tenantFromOpts(opts); tid != "" && a.TenantID != tid {
		return entity.FinancialAccount{}, repo.ErrNotFound
	}
	return a, nil
}

func (r *fakeAccountRepo) List(ctx context.Context, opts ...repo.Option) ([]entity.FinancialAccount, error) {
	r.calls.bump()
	r.mu.Lock()
	defer r.mu.Unlock()
	o := repo.ApplyOptions(opts...)
	out := make([]entity.FinancialAccount, 0, len(r.rows))
	for _, a := range r.rows {
		if o.TenantID != "" && a.TenantID != o.TenantID {
			continue
		}
		if !matchAccountFilter(a, o.Filters) {
			continue
		}
		out = append(out, a)
	}
	sortByCreatedThenIDAccounts(out)
	if o.Limit > 0 && len(out) > o.Limit {
		out = out[:o.Limit]
	}
	return out, nil
}

func matchAccountFilter(a entity.FinancialAccount, filters []repo.Filter) bool {
	for _, f := range filters {
		if f.Op != "=" {
			return false
		}
		want, ok := f.Value.(string)
		if !ok {
			return false
		}
		switch f.Field {
		case "name":
			if a.Name != want {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func (r *fakeAccountRepo) Create(ctx context.Context, a entity.FinancialAccount, opts ...repo.Option) (entity.FinancialAccount, error) {
	r.calls.bump()
	r.mu.Lock()
	defer r.mu.Unlock()
	if a.ID == "" {
		r.nextID++
		a.ID = "acc-" + strconv.Itoa(r.nextID)
	}
	ts := testBase.Add(time.Duration(r.nextID) * time.Second)
	a.CreatedAt = ts
	a.UpdatedAt = ts
	if tid := tenantFromOpts(opts); tid != "" {
		a.TenantID = tid
	}
	r.rows[a.ID] = a
	return a, nil
}

func (r *fakeAccountRepo) Update(ctx context.Context, a entity.FinancialAccount, opts ...repo.Option) (entity.FinancialAccount, error) {
	r.calls.bump()
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.rows[a.ID]; !ok {
		return entity.FinancialAccount{}, repo.ErrNotFound
	}
	if tid := tenantFromOpts(opts); tid != "" && a.TenantID != tid {
		return entity.FinancialAccount{}, repo.ErrNotFound
	}
	r.rows[a.ID] = a
	return a, nil
}

func (r *fakeAccountRepo) Delete(ctx context.Context, id string, opts ...repo.Option) error {
	r.calls.bump()
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.rows[id]
	if !ok {
		return repo.ErrNotFound
	}
	if tid := tenantFromOpts(opts); tid != "" && a.TenantID != tid {
		return repo.ErrNotFound
	}
	delete(r.rows, id)
	return nil
}

// count returns the number of stored accounts.
func (r *fakeAccountRepo) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.rows)
}

// get returns the account with the given ID.
func (r *fakeAccountRepo) get(id string) (entity.FinancialAccount, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.rows[id]
	return a, ok
}

// all returns every stored account in (created_at, id) order.
func (r *fakeAccountRepo) all() []entity.FinancialAccount {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]entity.FinancialAccount, 0, len(r.rows))
	for _, a := range r.rows {
		out = append(out, a)
	}
	sortByCreatedThenIDAccounts(out)
	return out
}

func sortByCreatedThenIDAccounts(out []entity.FinancialAccount) {
	sort.Slice(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.Before(out[j].CreatedAt)
		}
		return out[i].ID < out[j].ID
	})
}

// fakeMovementRepo is an in-memory implementation of
// repo.Repository[entity.MoneyMovement] with shared call counting.
type fakeMovementRepo struct {
	mu     sync.Mutex
	rows   map[string]entity.MoneyMovement
	nextID int
	calls  *callCounter
}

func (r *fakeMovementRepo) Get(ctx context.Context, id string, opts ...repo.Option) (entity.MoneyMovement, error) {
	r.calls.bump()
	r.mu.Lock()
	defer r.mu.Unlock()
	m, ok := r.rows[id]
	if !ok {
		return entity.MoneyMovement{}, repo.ErrNotFound
	}
	if tid := tenantFromOpts(opts); tid != "" && m.TenantID != tid {
		return entity.MoneyMovement{}, repo.ErrNotFound
	}
	return m, nil
}

func (r *fakeMovementRepo) List(ctx context.Context, opts ...repo.Option) ([]entity.MoneyMovement, error) {
	r.calls.bump()
	r.mu.Lock()
	defer r.mu.Unlock()
	o := repo.ApplyOptions(opts...)
	out := make([]entity.MoneyMovement, 0, len(r.rows))
	for _, m := range r.rows {
		if o.TenantID != "" && m.TenantID != o.TenantID {
			continue
		}
		if !matchMovementFilter(m, o.Filters) {
			continue
		}
		out = append(out, m)
	}
	if strings.HasPrefix(o.OrderBy, "created_at") {
		sortByCreatedThenIDMovements(out)
	} else {
		sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	}
	if o.Limit > 0 && len(out) > o.Limit {
		out = out[:o.Limit]
	}
	return out, nil
}

// matchMovementFilter reports whether m satisfies all filters. Supported "="
// fields: linked_document_id, source_account_id, destination_account_id
// (pointer fields; nil never matches). Unsupported field or operator is false.
func matchMovementFilter(m entity.MoneyMovement, filters []repo.Filter) bool {
	for _, f := range filters {
		if f.Op != "=" {
			return false
		}
		want, ok := f.Value.(string)
		if !ok {
			return false
		}
		var got *string
		switch f.Field {
		case "linked_document_id":
			got = m.LinkedDocumentID
		case "source_account_id":
			got = m.SourceAccountID
		case "destination_account_id":
			got = m.DestinationAccountID
		default:
			return false
		}
		if got == nil || *got != want {
			return false
		}
	}
	return true
}

func (r *fakeMovementRepo) Create(ctx context.Context, m entity.MoneyMovement, opts ...repo.Option) (entity.MoneyMovement, error) {
	r.calls.bump()
	r.mu.Lock()
	defer r.mu.Unlock()
	if m.ID == "" {
		r.nextID++
		m.ID = "mv-" + strconv.Itoa(r.nextID)
	}
	ts := testBase.Add(time.Duration(r.nextID) * time.Second)
	m.CreatedAt = ts
	m.UpdatedAt = ts
	m.RecordedAt = ts
	if tid := tenantFromOpts(opts); tid != "" {
		m.TenantID = tid
	}
	r.rows[m.ID] = m
	return m, nil
}

func (r *fakeMovementRepo) Update(ctx context.Context, m entity.MoneyMovement, opts ...repo.Option) (entity.MoneyMovement, error) {
	r.calls.bump()
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.rows[m.ID]; !ok {
		return entity.MoneyMovement{}, repo.ErrNotFound
	}
	if tid := tenantFromOpts(opts); tid != "" && m.TenantID != tid {
		return entity.MoneyMovement{}, repo.ErrNotFound
	}
	r.rows[m.ID] = m
	return m, nil
}

func (r *fakeMovementRepo) Delete(ctx context.Context, id string, opts ...repo.Option) error {
	r.calls.bump()
	r.mu.Lock()
	defer r.mu.Unlock()
	m, ok := r.rows[id]
	if !ok {
		return repo.ErrNotFound
	}
	if tid := tenantFromOpts(opts); tid != "" && m.TenantID != tid {
		return repo.ErrNotFound
	}
	delete(r.rows, id)
	return nil
}

// count returns the number of stored movements.
func (r *fakeMovementRepo) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.rows)
}

// get returns the movement with the given ID.
func (r *fakeMovementRepo) get(id string) (entity.MoneyMovement, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	m, ok := r.rows[id]
	return m, ok
}

// all returns every stored movement in (created_at, id) order.
func (r *fakeMovementRepo) all() []entity.MoneyMovement {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]entity.MoneyMovement, 0, len(r.rows))
	for _, m := range r.rows {
		out = append(out, m)
	}
	sortByCreatedThenIDMovements(out)
	return out
}

func sortByCreatedThenIDMovements(out []entity.MoneyMovement) {
	sort.Slice(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.Before(out[j].CreatedAt)
		}
		return out[i].ID < out[j].ID
	})
}

// fakeDocumentRepo is an in-memory implementation of
// repo.Repository[entity.Document] with shared call counting.
type fakeDocumentRepo struct {
	mu     sync.Mutex
	rows   map[string]entity.Document
	nextID int
	calls  *callCounter
}

func (r *fakeDocumentRepo) Get(ctx context.Context, id string, opts ...repo.Option) (entity.Document, error) {
	r.calls.bump()
	r.mu.Lock()
	defer r.mu.Unlock()
	d, ok := r.rows[id]
	if !ok {
		return entity.Document{}, repo.ErrNotFound
	}
	if tid := tenantFromOpts(opts); tid != "" && d.TenantID != tid {
		return entity.Document{}, repo.ErrNotFound
	}
	return d, nil
}

func (r *fakeDocumentRepo) List(ctx context.Context, opts ...repo.Option) ([]entity.Document, error) {
	r.calls.bump()
	r.mu.Lock()
	defer r.mu.Unlock()
	o := repo.ApplyOptions(opts...)
	out := make([]entity.Document, 0, len(r.rows))
	for _, d := range r.rows {
		if o.TenantID != "" && d.TenantID != o.TenantID {
			continue
		}
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	if o.Limit > 0 && len(out) > o.Limit {
		out = out[:o.Limit]
	}
	return out, nil
}

func (r *fakeDocumentRepo) Create(ctx context.Context, d entity.Document, opts ...repo.Option) (entity.Document, error) {
	r.calls.bump()
	r.mu.Lock()
	defer r.mu.Unlock()
	if d.ID == "" {
		r.nextID++
		d.ID = "doc-" + strconv.Itoa(r.nextID)
	}
	if tid := tenantFromOpts(opts); tid != "" {
		d.TenantID = tid
	}
	d.ExtractedFields = copyMapAny(d.ExtractedFields)
	d.CreatedAt = testBase.Add(time.Duration(r.nextID) * time.Second)
	r.rows[d.ID] = d
	return d, nil
}

func (r *fakeDocumentRepo) Update(ctx context.Context, d entity.Document, opts ...repo.Option) (entity.Document, error) {
	r.calls.bump()
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.rows[d.ID]; !ok {
		return entity.Document{}, repo.ErrNotFound
	}
	if tid := tenantFromOpts(opts); tid != "" && d.TenantID != tid {
		return entity.Document{}, repo.ErrNotFound
	}
	d.ExtractedFields = copyMapAny(d.ExtractedFields)
	r.rows[d.ID] = d
	return d, nil
}

func (r *fakeDocumentRepo) Delete(ctx context.Context, id string, opts ...repo.Option) error {
	r.calls.bump()
	r.mu.Lock()
	defer r.mu.Unlock()
	d, ok := r.rows[id]
	if !ok {
		return repo.ErrNotFound
	}
	if tid := tenantFromOpts(opts); tid != "" && d.TenantID != tid {
		return repo.ErrNotFound
	}
	delete(r.rows, id)
	return nil
}

// count returns the number of stored documents.
func (r *fakeDocumentRepo) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.rows)
}

// get returns the document with the given ID.
func (r *fakeDocumentRepo) get(id string) (entity.Document, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	d, ok := r.rows[id]
	return d, ok
}

// all returns every stored document in ID order.
func (r *fakeDocumentRepo) all() []entity.Document {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]entity.Document, 0, len(r.rows))
	for _, d := range r.rows {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// ---------------------------------------------------------------------------
// fake balancer
// ---------------------------------------------------------------------------

// fakeBalancer implements BalanceQuerier over the shared fake movement repo,
// using exact decimal math via math/big (no floats).
type fakeBalancer struct {
	movements *fakeMovementRepo
}

func (b *fakeBalancer) BalanceForAccount(ctx context.Context, accountID string, opts ...repo.Option) (string, error) {
	// Tenant scoping flows through opts into the movements list.
	all, err := b.movements.List(ctx, opts...)
	if err != nil {
		return "", err
	}
	// Use big.Int with scale-2 fixed point (cents).
	sum := new(big.Int)
	for _, m := range all {
		v := decimalToScaled(m.Amount) // big.Int in cents
		if m.DestinationAccountID != nil && *m.DestinationAccountID == accountID {
			sum.Add(sum, v)
		}
		if m.SourceAccountID != nil && *m.SourceAccountID == accountID {
			sum.Sub(sum, v)
		}
	}
	return scaledToString(sum), nil
}

// decimalToScaled converts an exact-decimal string like "19999.99" to a
// big.Int representing the value times 100 (cents). Fractional parts longer
// than two digits are truncated to cents. Tests never feed invalid amounts.
func decimalToScaled(s string) *big.Int {
	intPart, fracPart := s, ""
	if i := strings.IndexByte(s, '.'); i >= 0 {
		intPart, fracPart = s[:i], s[i+1:]
	}
	for len(fracPart) < 2 {
		fracPart += "0"
	}
	if len(fracPart) > 2 {
		fracPart = fracPart[:2]
	}
	intVal := new(big.Int)
	if intPart != "" {
		intVal.SetString(intPart, 10)
	}
	fracVal := new(big.Int)
	fracVal.SetString(fracPart, 10)
	result := new(big.Int).Mul(intVal, big.NewInt(100))
	result.Add(result, fracVal)
	return result
}

// scaledToString formats a big.Int in cents back to a decimal string with no
// trailing zeros or dot: "0", "150", "-100", "149.5".
func scaledToString(v *big.Int) string {
	sign := ""
	if v.Sign() < 0 {
		sign = "-"
		v = new(big.Int).Neg(v)
	}
	whole := new(big.Int).Quo(v, big.NewInt(100))
	frac := new(big.Int).Rem(v, big.NewInt(100))
	if whole.Sign() == 0 && frac.Sign() == 0 {
		return "0"
	}
	if frac.Sign() == 0 {
		return sign + whole.String()
	}
	fracStr := frac.String()
	if len(fracStr) == 1 {
		fracStr = "0" + fracStr
	}
	fracStr = strings.TrimRight(fracStr, "0")
	return sign + whole.String() + "." + fracStr
}

// ---------------------------------------------------------------------------
// shared helpers
// ---------------------------------------------------------------------------

// tenantFromOpts extracts the tenant ID from options ("" if absent).
func tenantFromOpts(opts []repo.Option) string {
	return repo.ApplyOptions(opts...).TenantID
}

// copyMapAny shallow-copies a map (nil-safe).
func copyMapAny(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// createAccount is a test helper: create an account and return its ID.
func (e *testEnv) createAccount(t *testing.T, ctx context.Context, name, typ, currency string) string {
	t.Helper()
	acc, err := e.svc.CreateAccount(ctx, AccountInput{Name: name, Type: typ, Currency: currency})
	if err != nil {
		t.Fatalf("CreateAccount(%q) failed: %v", name, err)
	}
	return acc.ID
}

// createDoc is a test helper: create a document directly in the fake and
// return its ID. The tenant option mirrors how document ingestion persists
// documents (the repo scopes by option, not context).
func (e *testEnv) createDoc(t *testing.T, ctx context.Context, fields map[string]any) string {
	t.Helper()
	tid, err := tenant.TenantFrom(ctx)
	if err != nil {
		t.Fatalf("createDoc: ctx has no tenant: %v", err)
	}
	d, err := e.docs.Create(ctx, entity.Document{ExtractedFields: fields}, repo.Tenant(tid))
	if err != nil {
		t.Fatalf("doc Create failed: %v", err)
	}
	return d.ID
}

// seedMovement inserts a movement directly into the fake (bypassing the
// service) for import-origin scenarios. Returns the movement ID.
func (e *testEnv) seedMovement(t *testing.T, m entity.MoneyMovement) string {
	t.Helper()
	e.movements.mu.Lock()
	defer e.movements.mu.Unlock()
	e.movements.calls.bump()
	if m.ID == "" {
		e.movements.nextID++
		m.ID = "mv-" + strconv.Itoa(e.movements.nextID)
	}
	ts := testBase.Add(time.Duration(e.movements.nextID) * time.Second)
	m.CreatedAt = ts
	m.UpdatedAt = ts
	if m.RecordedAt.IsZero() {
		m.RecordedAt = ts
	}
	e.movements.rows[m.ID] = m
	return m.ID
}

// balanceOf is a test helper: get the balance string for an account via the
// service's balancer (same code path as GetAccount).
func (e *testEnv) balanceOf(t *testing.T, ctx context.Context, accountID string) string {
	t.Helper()
	_, bal, err := e.svc.GetAccount(ctx, accountID)
	if err != nil {
		t.Fatalf("GetAccount(%q) failed: %v", accountID, err)
	}
	return bal
}

// movementCore is a snapshot of the immutable core fields of a movement.
type movementCore struct {
	Amount               string
	Currency             string
	OccurredOn           time.Time
	Kind                 string
	SourceAccountID      *string
	DestinationAccountID *string
}

func coreOf(m entity.MoneyMovement) movementCore {
	return movementCore{
		Amount:               m.Amount,
		Currency:             m.Currency,
		OccurredOn:           m.OccurredOn,
		Kind:                 m.Kind,
		SourceAccountID:      m.SourceAccountID,
		DestinationAccountID: m.DestinationAccountID,
	}
}

func assertCoreUnchanged(t *testing.T, label string, before, after movementCore) {
	t.Helper()
	if before.Amount != after.Amount {
		t.Errorf("%s: Amount changed: %q -> %q", label, before.Amount, after.Amount)
	}
	if before.Currency != after.Currency {
		t.Errorf("%s: Currency changed: %q -> %q", label, before.Currency, after.Currency)
	}
	if !before.OccurredOn.Equal(after.OccurredOn) {
		t.Errorf("%s: OccurredOn changed: %v -> %v", label, before.OccurredOn, after.OccurredOn)
	}
	if before.Kind != after.Kind {
		t.Errorf("%s: Kind changed: %q -> %q", label, before.Kind, after.Kind)
	}
	if !ptrEq(before.SourceAccountID, after.SourceAccountID) {
		t.Errorf("%s: SourceAccountID changed", label)
	}
	if !ptrEq(before.DestinationAccountID, after.DestinationAccountID) {
		t.Errorf("%s: DestinationAccountID changed", label)
	}
}

func ptrEq(a, b *string) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestCreateAccountPersistsFullFieldSet — a fully-populated account round-trips.
func TestCreateAccountPersistsFullFieldSet(t *testing.T) {
	t.Parallel()
	e := newTestEnv()
	ctx := ctxWithTenant(testTenant)

	acc, err := e.svc.CreateAccount(ctx, AccountInput{
		Name:               "HDFC Savings",
		Type:               entity.AccountTypeBank,
		Currency:           "INR",
		Institution:        strPtr("HDFC Bank"),
		ExternalDescriptor: strPtr("XX1234"),
	})
	if err != nil {
		t.Fatalf("CreateAccount failed: %v", err)
	}
	if acc.ID == "" {
		t.Fatal("created account has empty ID")
	}

	got, balance, err := e.svc.GetAccount(ctx, acc.ID)
	if err != nil {
		t.Fatalf("GetAccount failed: %v", err)
	}
	if got.Name != "HDFC Savings" {
		t.Errorf("Name = %q, want %q", got.Name, "HDFC Savings")
	}
	if got.Type != entity.AccountTypeBank {
		t.Errorf("Type = %q, want %q", got.Type, entity.AccountTypeBank)
	}
	if got.Currency != "INR" {
		t.Errorf("Currency = %q, want %q", got.Currency, "INR")
	}
	if got.Institution == nil || *got.Institution != "HDFC Bank" {
		t.Errorf("Institution = %v, want %q", got.Institution, "HDFC Bank")
	}
	if got.ExternalDescriptor == nil || *got.ExternalDescriptor != "XX1234" {
		t.Errorf("ExternalDescriptor = %v, want %q", got.ExternalDescriptor, "XX1234")
	}
	if balance != "0" {
		t.Errorf("balance = %q, want %q", balance, "0")
	}
}

// TestCreateAccountRejected — invalid inputs are rejected, nothing persisted.
func TestCreateAccountRejected(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		input AccountInput
	}{
		{"invalid type", AccountInput{Name: "X", Type: "investment", Currency: "INR"}},
		{"blank name", AccountInput{Name: "", Type: entity.AccountTypeBank, Currency: "INR"}},
		{"whitespace name", AccountInput{Name: "   ", Type: entity.AccountTypeBank, Currency: "INR"}},
		{"currency 2 chars", AccountInput{Name: "X", Type: entity.AccountTypeBank, Currency: "IN"}},
		{"currency 4 chars", AccountInput{Name: "X", Type: entity.AccountTypeBank, Currency: "INR1"}},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := newTestEnv()
			ctx := ctxWithTenant(testTenant)
			_, err := e.svc.CreateAccount(ctx, tc.input)
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("CreateAccount returned %v, want ErrInvalid", err)
			}
			if got := e.accounts.count(); got != 0 {
				t.Fatalf("account count = %d, want 0", got)
			}
		})
	}
}

// TestAccountCurrencyImmutableByConstruction — the exported method set of
// *Service is exactly the 10 specified methods; no account-mutation method exists.
func TestAccountCurrencyImmutableByConstruction(t *testing.T) {
	t.Parallel()

	want := []string{
		"CreateAccount",
		"CreateManualMovement",
		"DeleteMovement",
		"GetAccount",
		"GetMovement",
		"Link",
		"ListAccounts",
		"ListMovements",
		"PatchDescription",
		"Unlink",
	}
	sort.Strings(want)

	typ := reflect.TypeOf(&Service{})
	var got []string
	for i := 0; i < typ.NumMethod(); i++ {
		m := typ.Method(i)
		if m.IsExported() {
			got = append(got, m.Name)
		}
	}
	sort.Strings(got)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("exported method set = %v, want %v", got, want)
	}

	// Also: create an INR account, assert currency is still INR via GetAccount.
	e := newTestEnv()
	ctx := ctxWithTenant(testTenant)
	accID := e.createAccount(t, ctx, "A", entity.AccountTypeBank, "INR")
	gotAcc, _, err := e.svc.GetAccount(ctx, accID)
	if err != nil {
		t.Fatalf("GetAccount failed: %v", err)
	}
	if gotAcc.Currency != "INR" {
		t.Fatalf("Currency = %q, want INR", gotAcc.Currency)
	}
}

// TestDuplicateAccountNamesAllowed — two accounts with the same name get
// distinct IDs.
func TestDuplicateAccountNamesAllowed(t *testing.T) {
	t.Parallel()
	e := newTestEnv()
	ctx := ctxWithTenant(testTenant)

	id1 := e.createAccount(t, ctx, "Cash", entity.AccountTypeCash, "INR")
	id2 := e.createAccount(t, ctx, "Cash", entity.AccountTypeCash, "INR")
	if id1 == id2 {
		t.Fatalf("duplicate names produced same ID %q", id1)
	}
	all, err := e.svc.ListAccounts(ctx)
	if err != nil {
		t.Fatalf("ListAccounts failed: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("ListAccounts returned %d, want 2", len(all))
	}
}

// TestCreateExpenseMovementPersisted — an expense round-trips all fields.
func TestCreateExpenseMovementPersisted(t *testing.T) {
	t.Parallel()
	e := newTestEnv()
	ctx := ctxWithTenant(testTenant)
	a := e.createAccount(t, ctx, "A", entity.AccountTypeBank, "INR")

	occurred := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	mv, err := e.svc.CreateManualMovement(ctx, MovementInput{
		Kind:            entity.KindExpense,
		Amount:          "19999.99",
		Currency:        "INR",
		OccurredOn:      occurred,
		Description:     "  Reliance   Digital ",
		SourceAccountID: a,
	})
	if err != nil {
		t.Fatalf("CreateManualMovement failed: %v", err)
	}

	got, err := e.svc.GetMovement(ctx, mv.ID)
	if err != nil {
		t.Fatalf("GetMovement failed: %v", err)
	}
	if got.Kind != entity.KindExpense {
		t.Errorf("Kind = %q, want %q", got.Kind, entity.KindExpense)
	}
	if got.Amount != "19999.99" {
		t.Errorf("Amount = %q, want %q", got.Amount, "19999.99")
	}
	if got.Currency != "INR" {
		t.Errorf("Currency = %q, want %q", got.Currency, "INR")
	}
	if !got.OccurredOn.Equal(occurred) {
		t.Errorf("OccurredOn = %v, want %v", got.OccurredOn, occurred)
	}
	if got.Description != "  Reliance   Digital " {
		t.Errorf("Description = %q, want verbatim %q", got.Description, "  Reliance   Digital ")
	}
	if got.NormDescription != "reliance digital" {
		t.Errorf("NormDescription = %q, want %q", got.NormDescription, "reliance digital")
	}
	if got.SourceAccountID == nil || *got.SourceAccountID != a {
		t.Errorf("SourceAccountID = %v, want %q", got.SourceAccountID, a)
	}
	if got.DestinationAccountID != nil {
		t.Errorf("DestinationAccountID = %v, want nil", *got.DestinationAccountID)
	}
	if got.Origin != entity.OriginManual {
		t.Errorf("Origin = %q, want %q", got.Origin, entity.OriginManual)
	}
}

// TestCreateTransferRequiresDistinctSameCurrencyAccounts — A→A and
// A(INR)→B(USD) are both rejected; nothing persisted.
func TestCreateTransferRequiresDistinctSameCurrencyAccounts(t *testing.T) {
	t.Parallel()

	t.Run("same account source and dest", func(t *testing.T) {
		t.Parallel()
		e := newTestEnv()
		ctx := ctxWithTenant(testTenant)
		a := e.createAccount(t, ctx, "A", entity.AccountTypeBank, "INR")

		_, err := e.svc.CreateManualMovement(ctx, MovementInput{
			Kind:                 entity.KindTransfer,
			Amount:               "100",
			Currency:             "INR",
			OccurredOn:           time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
			Description:          "self",
			SourceAccountID:      a,
			DestinationAccountID: a,
		})
		if !errors.Is(err, ErrInvalid) {
			t.Fatalf("CreateManualMovement returned %v, want ErrInvalid", err)
		}
		if got := e.movements.count(); got != 0 {
			t.Fatalf("movement count = %d, want 0", got)
		}
	})

	t.Run("different currency", func(t *testing.T) {
		t.Parallel()
		e := newTestEnv()
		ctx := ctxWithTenant(testTenant)
		a := e.createAccount(t, ctx, "A", entity.AccountTypeBank, "INR")
		b := e.createAccount(t, ctx, "B", entity.AccountTypeBank, "USD")

		_, err := e.svc.CreateManualMovement(ctx, MovementInput{
			Kind:                 entity.KindTransfer,
			Amount:               "100",
			Currency:             "INR",
			OccurredOn:           time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
			Description:          "cross-currency",
			SourceAccountID:      a,
			DestinationAccountID: b,
		})
		if !errors.Is(err, ErrInvalid) {
			t.Fatalf("CreateManualMovement returned %v, want ErrInvalid", err)
		}
		if got := e.movements.count(); got != 0 {
			t.Fatalf("movement count = %d, want 0", got)
		}
	})
}

// TestCreateMovementRejectedInvalidFields — invalid fields are rejected.
func TestCreateMovementRejectedInvalidFields(t *testing.T) {
	t.Parallel()

	occurred := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)

	cases := []struct {
		name string
		mut  func(in *MovementInput)
	}{
		{"amount zero", func(in *MovementInput) { in.Amount = "0" }},
		{"amount negative", func(in *MovementInput) { in.Amount = "-50" }},
		{"amount bad dot", func(in *MovementInput) { in.Amount = "12.5.3" }},
		{"amount exponent", func(in *MovementInput) { in.Amount = "1e3" }},
		{"amount empty", func(in *MovementInput) { in.Amount = "" }},
		{"currency bad", func(in *MovementInput) { in.Currency = "XX1" }},
		{"description empty", func(in *MovementInput) { in.Description = "" }},
		{"description whitespace", func(in *MovementInput) { in.Description = "   " }},
		{"kind debit", func(in *MovementInput) { in.Kind = "debit" }},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := newTestEnv()
			ctx := ctxWithTenant(testTenant)
			a := e.createAccount(t, ctx, "A", entity.AccountTypeBank, "INR")

			in := MovementInput{
				Kind:            entity.KindExpense,
				Amount:          "100",
				Currency:        "INR",
				OccurredOn:      occurred,
				Description:     "test",
				SourceAccountID: a,
			}
			tc.mut(&in)

			_, err := e.svc.CreateManualMovement(ctx, in)
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("CreateManualMovement returned %v, want ErrInvalid", err)
			}
			if got := e.movements.count(); got != 0 {
				t.Fatalf("movement count = %d, want 0", got)
			}
		})
	}
}

// TestCreateIncomeMovementDestinationOnly — income has dest only.
func TestCreateIncomeMovementDestinationOnly(t *testing.T) {
	t.Parallel()
	e := newTestEnv()
	ctx := ctxWithTenant(testTenant)
	a := e.createAccount(t, ctx, "A", entity.AccountTypeBank, "INR")

	mv, err := e.svc.CreateManualMovement(ctx, MovementInput{
		Kind:                 entity.KindIncome,
		Amount:               "75000",
		Currency:             "INR",
		OccurredOn:           time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		Description:          "salary",
		DestinationAccountID: a,
	})
	if err != nil {
		t.Fatalf("CreateManualMovement failed: %v", err)
	}
	got, err := e.svc.GetMovement(ctx, mv.ID)
	if err != nil {
		t.Fatalf("GetMovement failed: %v", err)
	}
	if got.DestinationAccountID == nil || *got.DestinationAccountID != a {
		t.Errorf("DestinationAccountID = %v, want %q", got.DestinationAccountID, a)
	}
	if got.SourceAccountID != nil {
		t.Errorf("SourceAccountID = %v, want nil", *got.SourceAccountID)
	}
}

// TestCreateMovementUnknownAccount — referencing a nonexistent account.
func TestCreateMovementUnknownAccount(t *testing.T) {
	t.Parallel()
	e := newTestEnv()
	ctx := ctxWithTenant(testTenant)

	_, err := e.svc.CreateManualMovement(ctx, MovementInput{
		Kind:            entity.KindExpense,
		Amount:          "100",
		Currency:        "INR",
		OccurredOn:      time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		Description:     "ghost",
		SourceAccountID: "ghost",
	})
	if !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("CreateManualMovement returned %v, want ErrNotFound", err)
	}
	if got := e.movements.count(); got != 0 {
		t.Fatalf("movement count = %d, want 0", got)
	}
}

// TestListMovementsFilterByAccount — AccountID filter matches source OR dest.
func TestListMovementsFilterByAccount(t *testing.T) {
	t.Parallel()
	e := newTestEnv()
	ctx := ctxWithTenant(testTenant)
	a := e.createAccount(t, ctx, "A", entity.AccountTypeBank, "INR")
	b := e.createAccount(t, ctx, "B", entity.AccountTypeBank, "INR")

	occ := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)

	// expense from A
	mvA, err := e.svc.CreateManualMovement(ctx, MovementInput{
		Kind: entity.KindExpense, Amount: "100", Currency: "INR",
		OccurredOn: occ, Description: "expA", SourceAccountID: a,
	})
	if err != nil {
		t.Fatalf("expense A: %v", err)
	}
	// expense from B
	mvb, err := e.svc.CreateManualMovement(ctx, MovementInput{
		Kind: entity.KindExpense, Amount: "200", Currency: "INR",
		OccurredOn: occ, Description: "expB", SourceAccountID: b,
	})
	if err != nil {
		t.Fatalf("expense B: %v", err)
	}
	// transfer A→B
	mvt, err := e.svc.CreateManualMovement(ctx, MovementInput{
		Kind: entity.KindTransfer, Amount: "300", Currency: "INR",
		OccurredOn: occ, Description: "trans", SourceAccountID: a, DestinationAccountID: b,
	})
	if err != nil {
		t.Fatalf("transfer: %v", err)
	}

	// filter A → expense(A) + transfer
	gotA, err := e.svc.ListMovements(ctx, MovementListFilter{AccountID: a})
	if err != nil {
		t.Fatalf("ListMovements(A) failed: %v", err)
	}
	if len(gotA) != 2 {
		t.Fatalf("ListMovements(A) returned %d, want 2", len(gotA))
	}
	idsA := map[string]bool{}
	for _, m := range gotA {
		idsA[m.ID] = true
	}
	if !idsA[mvA.ID] || !idsA[mvt.ID] {
		t.Errorf("ListMovements(A) IDs = %v, want %v and %v", idsA, mvA.ID, mvt.ID)
	}

	// filter B → expense(B) + transfer
	gotB, err := e.svc.ListMovements(ctx, MovementListFilter{AccountID: b})
	if err != nil {
		t.Fatalf("ListMovements(B) failed: %v", err)
	}
	if len(gotB) != 2 {
		t.Fatalf("ListMovements(B) returned %d, want 2", len(gotB))
	}
	idsB := map[string]bool{}
	for _, m := range gotB {
		idsB[m.ID] = true
	}
	if !idsB[mvb.ID] || !idsB[mvt.ID] {
		t.Errorf("ListMovements(B) IDs = %v, want %v and %v", idsB, mvb.ID, mvt.ID)
	}

	// no filter → all 3
	gotAll, err := e.svc.ListMovements(ctx, MovementListFilter{})
	if err != nil {
		t.Fatalf("ListMovements() failed: %v", err)
	}
	if len(gotAll) != 3 {
		t.Fatalf("ListMovements() returned %d, want 3", len(gotAll))
	}
}

// TestListMovementsFilterByOccurredRange — inclusive date range filter.
func TestListMovementsFilterByOccurredRange(t *testing.T) {
	t.Parallel()
	e := newTestEnv()
	ctx := ctxWithTenant(testTenant)
	a := e.createAccount(t, ctx, "A", entity.AccountTypeBank, "INR")

	mvEarly, err := e.svc.CreateManualMovement(ctx, MovementInput{
		Kind: entity.KindExpense, Amount: "100", Currency: "INR",
		OccurredOn:  time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		Description: "early", SourceAccountID: a,
	})
	if err != nil {
		t.Fatalf("early: %v", err)
	}
	mvLate, err := e.svc.CreateManualMovement(ctx, MovementInput{
		Kind: entity.KindExpense, Amount: "200", Currency: "INR",
		OccurredOn:  time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC),
		Description: "late", SourceAccountID: a,
	})
	if err != nil {
		t.Fatalf("late: %v", err)
	}

	from := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)

	// from+to → only late (08-20)
	got, err := e.svc.ListMovements(ctx, MovementListFilter{OccurredFrom: &from, OccurredTo: &to})
	if err != nil {
		t.Fatalf("ListMovements(range) failed: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("ListMovements(range) returned %d, want 1", len(got))
	}
	if got[0].ID != mvLate.ID {
		t.Fatalf("ListMovements(range) ID = %q, want %q", got[0].ID, mvLate.ID)
	}

	// from-only (inclusive: movement ON the from day is included)
	fromDay := time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC)
	gotFrom, err := e.svc.ListMovements(ctx, MovementListFilter{OccurredFrom: &fromDay})
	if err != nil {
		t.Fatalf("ListMovements(from) failed: %v", err)
	}
	if len(gotFrom) != 1 {
		t.Fatalf("ListMovements(from) returned %d, want 1", len(gotFrom))
	}
	if gotFrom[0].ID != mvLate.ID {
		t.Fatalf("ListMovements(from) ID = %q, want %q", gotFrom[0].ID, mvLate.ID)
	}

	// to-only (inclusive: movement ON the to day is included)
	toDay := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	gotTo, err := e.svc.ListMovements(ctx, MovementListFilter{OccurredTo: &toDay})
	if err != nil {
		t.Fatalf("ListMovements(to) failed: %v", err)
	}
	if len(gotTo) != 1 {
		t.Fatalf("ListMovements(to) returned %d, want 1", len(gotTo))
	}
	if gotTo[0].ID != mvEarly.ID {
		t.Fatalf("ListMovements(to) ID = %q, want %q", gotTo[0].ID, mvEarly.ID)
	}
}

// TestListMovementsOrdering — creation order preserved.
func TestListMovementsOrdering(t *testing.T) {
	t.Parallel()
	e := newTestEnv()
	ctx := ctxWithTenant(testTenant)
	a := e.createAccount(t, ctx, "A", entity.AccountTypeBank, "INR")
	occ := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)

	id1, _ := e.svc.CreateManualMovement(ctx, MovementInput{
		Kind: entity.KindExpense, Amount: "1", Currency: "INR",
		OccurredOn: occ, Description: "m1", SourceAccountID: a,
	})
	id2, _ := e.svc.CreateManualMovement(ctx, MovementInput{
		Kind: entity.KindExpense, Amount: "2", Currency: "INR",
		OccurredOn: occ, Description: "m2", SourceAccountID: a,
	})
	id3, _ := e.svc.CreateManualMovement(ctx, MovementInput{
		Kind: entity.KindExpense, Amount: "3", Currency: "INR",
		OccurredOn: occ, Description: "m3", SourceAccountID: a,
	})

	got, err := e.svc.ListMovements(ctx, MovementListFilter{})
	if err != nil {
		t.Fatalf("ListMovements failed: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("ListMovements returned %d, want 3", len(got))
	}
	if got[0].ID != id1.ID || got[1].ID != id2.ID || got[2].ID != id3.ID {
		t.Fatalf("order = [%q, %q, %q], want [%q, %q, %q]",
			got[0].ID, got[1].ID, got[2].ID, id1.ID, id2.ID, id3.ID)
	}
}

// TestListAccountsOrdering — creation order preserved.
func TestListAccountsOrdering(t *testing.T) {
	t.Parallel()
	e := newTestEnv()
	ctx := ctxWithTenant(testTenant)

	id1 := e.createAccount(t, ctx, "A", entity.AccountTypeBank, "INR")
	id2 := e.createAccount(t, ctx, "B", entity.AccountTypeBank, "INR")
	id3 := e.createAccount(t, ctx, "C", entity.AccountTypeBank, "INR")

	got, err := e.svc.ListAccounts(ctx)
	if err != nil {
		t.Fatalf("ListAccounts failed: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("ListAccounts returned %d, want 3", len(got))
	}
	if got[0].ID != id1 || got[1].ID != id2 || got[2].ID != id3 {
		t.Fatalf("order = [%q, %q, %q], want [%q, %q, %q]",
			got[0].ID, got[1].ID, got[2].ID, id1, id2, id3)
	}
}

// TestListEmptyResults — empty ledgers return non-nil empty slices.
func TestListEmptyResults(t *testing.T) {
	t.Parallel()
	e := newTestEnv()
	ctx := ctxWithTenant(testTenant)

	accs, err := e.svc.ListAccounts(ctx)
	if err != nil {
		t.Fatalf("ListAccounts failed: %v", err)
	}
	if accs == nil {
		t.Fatal("ListAccounts returned nil, want non-nil empty slice")
	}
	if len(accs) != 0 {
		t.Fatalf("ListAccounts returned %d, want 0", len(accs))
	}

	mvs, err := e.svc.ListMovements(ctx, MovementListFilter{})
	if err != nil {
		t.Fatalf("ListMovements failed: %v", err)
	}
	if mvs == nil {
		t.Fatal("ListMovements returned nil, want non-nil empty slice")
	}
	if len(mvs) != 0 {
		t.Fatalf("ListMovements returned %d, want 0", len(mvs))
	}
}

// TestBalanceAccumulates — income + expense.
func TestBalanceAccumulates(t *testing.T) {
	t.Parallel()
	e := newTestEnv()
	ctx := ctxWithTenant(testTenant)
	a := e.createAccount(t, ctx, "A", entity.AccountTypeBank, "INR")
	occ := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)

	_, err := e.svc.CreateManualMovement(ctx, MovementInput{
		Kind: entity.KindIncome, Amount: "250", Currency: "INR",
		OccurredOn: occ, Description: "income", DestinationAccountID: a,
	})
	if err != nil {
		t.Fatalf("income: %v", err)
	}
	_, err = e.svc.CreateManualMovement(ctx, MovementInput{
		Kind: entity.KindExpense, Amount: "100", Currency: "INR",
		OccurredOn: occ, Description: "expense", SourceAccountID: a,
	})
	if err != nil {
		t.Fatalf("expense: %v", err)
	}

	bal := e.balanceOf(t, ctx, a)
	if bal != "150" {
		t.Fatalf("balance = %q, want %q", bal, "150")
	}
}

// TestTransferMovesValue — transfer 100 A→B.
func TestTransferMovesValue(t *testing.T) {
	t.Parallel()
	e := newTestEnv()
	ctx := ctxWithTenant(testTenant)
	a := e.createAccount(t, ctx, "A", entity.AccountTypeBank, "INR")
	b := e.createAccount(t, ctx, "B", entity.AccountTypeBank, "INR")

	_, err := e.svc.CreateManualMovement(ctx, MovementInput{
		Kind: entity.KindTransfer, Amount: "100", Currency: "INR",
		OccurredOn:  time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		Description: "transfer", SourceAccountID: a, DestinationAccountID: b,
	})
	if err != nil {
		t.Fatalf("transfer: %v", err)
	}

	if bal := e.balanceOf(t, ctx, a); bal != "-100" {
		t.Errorf("A balance = %q, want %q", bal, "-100")
	}
	if bal := e.balanceOf(t, ctx, b); bal != "100" {
		t.Errorf("B balance = %q, want %q", bal, "100")
	}
}

// TestBalanceZeroWhenEmpty — fresh account has balance "0".
func TestBalanceZeroWhenEmpty(t *testing.T) {
	t.Parallel()
	e := newTestEnv()
	ctx := ctxWithTenant(testTenant)
	a := e.createAccount(t, ctx, "A", entity.AccountTypeBank, "INR")
	if bal := e.balanceOf(t, ctx, a); bal != "0" {
		t.Fatalf("balance = %q, want %q", bal, "0")
	}
}

// TestImportedMovementProvenance — import-origin fields are exposed;
// manual movements have them nil.
func TestImportedMovementProvenance(t *testing.T) {
	t.Parallel()
	e := newTestEnv()
	ctx := ctxWithTenant(testTenant)
	a := e.createAccount(t, ctx, "A", entity.AccountTypeBank, "INR")

	line := 3
	importedID := e.seedMovement(t, entity.MoneyMovement{
		TenantID:          testTenant,
		Kind:              entity.KindExpense,
		Amount:            "500",
		Currency:          "INR",
		OccurredOn:        time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		Description:       "imported",
		NormDescription:   "imported",
		Origin:            entity.OriginImport,
		SourceAccountID:   strPtr(a),
		ImportBatchID:     strPtr("batch-1"),
		ImportLine:        &line,
		ExternalReference: strPtr("EXT-9"),
	})

	got, err := e.svc.GetMovement(ctx, importedID)
	if err != nil {
		t.Fatalf("GetMovement(imported) failed: %v", err)
	}
	if got.ImportBatchID == nil || *got.ImportBatchID != "batch-1" {
		t.Errorf("ImportBatchID = %v, want %q", got.ImportBatchID, "batch-1")
	}
	if got.ImportLine == nil || *got.ImportLine != 3 {
		t.Errorf("ImportLine = %v, want 3", got.ImportLine)
	}
	if got.ExternalReference == nil || *got.ExternalReference != "EXT-9" {
		t.Errorf("ExternalReference = %v, want %q", got.ExternalReference, "EXT-9")
	}

	// manual movement has all three nil
	mv, err := e.svc.CreateManualMovement(ctx, MovementInput{
		Kind: entity.KindExpense, Amount: "100", Currency: "INR",
		OccurredOn:  time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC),
		Description: "manual", SourceAccountID: a,
	})
	if err != nil {
		t.Fatalf("CreateManualMovement failed: %v", err)
	}
	gotManual, err := e.svc.GetMovement(ctx, mv.ID)
	if err != nil {
		t.Fatalf("GetMovement(manual) failed: %v", err)
	}
	if gotManual.ImportBatchID != nil {
		t.Errorf("manual ImportBatchID = %v, want nil", *gotManual.ImportBatchID)
	}
	if gotManual.ImportLine != nil {
		t.Errorf("manual ImportLine = %v, want nil", *gotManual.ImportLine)
	}
	if gotManual.ExternalReference != nil {
		t.Errorf("manual ExternalReference = %v, want nil", *gotManual.ExternalReference)
	}
}

// TestLinkCreatedAndVisible — a link is created and visible via GetMovement.
func TestLinkCreatedAndVisible(t *testing.T) {
	t.Parallel()
	e := newTestEnv()
	ctx := ctxWithTenant(testTenant)
	a := e.createAccount(t, ctx, "A", entity.AccountTypeBank, "INR")

	mv, err := e.svc.CreateManualMovement(ctx, MovementInput{
		Kind: entity.KindExpense, Amount: "100", Currency: "INR",
		OccurredOn:  time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		Description: "test", SourceAccountID: a,
	})
	if err != nil {
		t.Fatalf("CreateManualMovement failed: %v", err)
	}
	docID := e.createDoc(t, ctx, nil)

	before := coreOf(mv)
	docBefore, _ := e.docs.get(docID)

	if err := e.svc.Link(ctx, mv.ID, docID, "manual"); err != nil {
		t.Fatalf("Link failed: %v", err)
	}

	got, err := e.svc.GetMovement(ctx, mv.ID)
	if err != nil {
		t.Fatalf("GetMovement failed: %v", err)
	}
	if got.LinkedDocumentID == nil || *got.LinkedDocumentID != docID {
		t.Errorf("LinkedDocumentID = %v, want %q", got.LinkedDocumentID, docID)
	}
	if got.LinkCreator == nil || *got.LinkCreator != "manual" {
		t.Errorf("LinkCreator = %v, want %q", got.LinkCreator, "manual")
	}
	if got.LinkConflicting {
		t.Error("LinkConflicting = true, want false")
	}

	// core fields unchanged
	assertCoreUnchanged(t, "after link", before, coreOf(got))

	// document unchanged
	docAfter, _ := e.docs.get(docID)
	if !reflect.DeepEqual(docBefore, docAfter) {
		t.Errorf("document changed: before=%+v after=%+v", docBefore, docAfter)
	}
}

// TestLinkIdempotentSameLink — linking the same pair twice is a no-op.
func TestLinkIdempotentSameLink(t *testing.T) {
	t.Parallel()
	e := newTestEnv()
	ctx := ctxWithTenant(testTenant)
	a := e.createAccount(t, ctx, "A", entity.AccountTypeBank, "INR")

	mv, _ := e.svc.CreateManualMovement(ctx, MovementInput{
		Kind: entity.KindExpense, Amount: "100", Currency: "INR",
		OccurredOn:  time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		Description: "test", SourceAccountID: a,
	})
	docID := e.createDoc(t, ctx, nil)

	if err := e.svc.Link(ctx, mv.ID, docID, "manual"); err != nil {
		t.Fatalf("first Link failed: %v", err)
	}
	mvCount := e.movements.count()

	if err := e.svc.Link(ctx, mv.ID, docID, "manual"); err != nil {
		t.Fatalf("second Link failed: %v, want nil (idempotent)", err)
	}

	got, _ := e.svc.GetMovement(ctx, mv.ID)
	if got.LinkedDocumentID == nil || *got.LinkedDocumentID != docID {
		t.Errorf("LinkedDocumentID = %v, want %q", got.LinkedDocumentID, docID)
	}
	if e.movements.count() != mvCount {
		t.Errorf("movement count changed: %d -> %d", mvCount, e.movements.count())
	}
}

// TestLinkAlreadyLinkedMovementConflicts — re-linking to a different doc.
func TestLinkAlreadyLinkedMovementConflicts(t *testing.T) {
	t.Parallel()
	e := newTestEnv()
	ctx := ctxWithTenant(testTenant)
	a := e.createAccount(t, ctx, "A", entity.AccountTypeBank, "INR")

	mv, _ := e.svc.CreateManualMovement(ctx, MovementInput{
		Kind: entity.KindExpense, Amount: "100", Currency: "INR",
		OccurredOn:  time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		Description: "test", SourceAccountID: a,
	})
	doc1 := e.createDoc(t, ctx, nil)
	doc2 := e.createDoc(t, ctx, nil)

	if err := e.svc.Link(ctx, mv.ID, doc1, "manual"); err != nil {
		t.Fatalf("Link to doc1 failed: %v", err)
	}

	err := e.svc.Link(ctx, mv.ID, doc2, "manual")
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("Link to doc2 returned %v, want ErrConflict", err)
	}

	got, _ := e.svc.GetMovement(ctx, mv.ID)
	if got.LinkedDocumentID == nil || *got.LinkedDocumentID != doc1 {
		t.Errorf("LinkedDocumentID = %v, want %q (unchanged)", got.LinkedDocumentID, doc1)
	}
}

// TestLinkAlreadyLinkedDocumentConflicts — linking a doc already linked to
// a different movement.
func TestLinkAlreadyLinkedDocumentConflicts(t *testing.T) {
	t.Parallel()
	e := newTestEnv()
	ctx := ctxWithTenant(testTenant)
	a := e.createAccount(t, ctx, "A", entity.AccountTypeBank, "INR")
	occ := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)

	m1, _ := e.svc.CreateManualMovement(ctx, MovementInput{
		Kind: entity.KindExpense, Amount: "100", Currency: "INR",
		OccurredOn: occ, Description: "m1", SourceAccountID: a,
	})
	m2, _ := e.svc.CreateManualMovement(ctx, MovementInput{
		Kind: entity.KindExpense, Amount: "200", Currency: "INR",
		OccurredOn: occ, Description: "m2", SourceAccountID: a,
	})
	doc1 := e.createDoc(t, ctx, nil)

	if err := e.svc.Link(ctx, m1.ID, doc1, "manual"); err != nil {
		t.Fatalf("Link m1→doc1 failed: %v", err)
	}

	err := e.svc.Link(ctx, m2.ID, doc1, "manual")
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("Link m2→doc1 returned %v, want ErrConflict", err)
	}

	// m1's link unchanged
	got1, _ := e.svc.GetMovement(ctx, m1.ID)
	if got1.LinkedDocumentID == nil || *got1.LinkedDocumentID != doc1 {
		t.Errorf("m1 LinkedDocumentID = %v, want %q", got1.LinkedDocumentID, doc1)
	}
	// m2 unlinked
	got2, _ := e.svc.GetMovement(ctx, m2.ID)
	if got2.LinkedDocumentID != nil {
		t.Errorf("m2 LinkedDocumentID = %v, want nil", *got2.LinkedDocumentID)
	}
}

// TestLinkCrossTenantDocumentNotFound — cross-tenant and unknown IDs.
func TestLinkCrossTenantDocumentNotFound(t *testing.T) {
	t.Parallel()
	e := newTestEnv()
	ctxA := ctxWithTenant(testTenant)
	ctxB := ctxWithTenant(testTenantB)

	a := e.createAccount(t, ctxA, "A", entity.AccountTypeBank, "INR")
	mv, _ := e.svc.CreateManualMovement(ctxA, MovementInput{
		Kind: entity.KindExpense, Amount: "100", Currency: "INR",
		OccurredOn:  time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		Description: "test", SourceAccountID: a,
	})

	// doc created under tenant B
	docB := e.createDoc(t, ctxB, nil)

	// cross-tenant doc → ErrNotFound
	err := e.svc.Link(ctxA, mv.ID, docB, "manual")
	if !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("Link cross-tenant doc returned %v, want ErrNotFound", err)
	}

	// unknown document → ErrNotFound
	err = e.svc.Link(ctxA, mv.ID, "ghost-doc", "manual")
	if !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("Link unknown doc returned %v, want ErrNotFound", err)
	}

	// unknown movement → ErrNotFound
	err = e.svc.Link(ctxA, "ghost-mv", docB, "manual")
	if !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("Link unknown movement returned %v, want ErrNotFound", err)
	}

	// no link created
	got, _ := e.svc.GetMovement(ctxA, mv.ID)
	if got.LinkedDocumentID != nil {
		t.Errorf("LinkedDocumentID = %v, want nil (no link created)", *got.LinkedDocumentID)
	}
}

// TestUnlinkRemovesLink — unlink clears the link; doc can be re-linked.
func TestUnlinkRemovesLink(t *testing.T) {
	t.Parallel()
	e := newTestEnv()
	ctx := ctxWithTenant(testTenant)
	a := e.createAccount(t, ctx, "A", entity.AccountTypeBank, "INR")
	occ := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)

	mv1, _ := e.svc.CreateManualMovement(ctx, MovementInput{
		Kind: entity.KindExpense, Amount: "100", Currency: "INR",
		OccurredOn: occ, Description: "m1", SourceAccountID: a,
	})
	mv2, _ := e.svc.CreateManualMovement(ctx, MovementInput{
		Kind: entity.KindExpense, Amount: "200", Currency: "INR",
		OccurredOn: occ, Description: "m2", SourceAccountID: a,
	})
	docID := e.createDoc(t, ctx, nil)

	if err := e.svc.Link(ctx, mv1.ID, docID, "manual"); err != nil {
		t.Fatalf("Link failed: %v", err)
	}
	if err := e.svc.Unlink(ctx, mv1.ID); err != nil {
		t.Fatalf("Unlink failed: %v", err)
	}

	got, _ := e.svc.GetMovement(ctx, mv1.ID)
	if got.LinkedDocumentID != nil {
		t.Errorf("LinkedDocumentID = %v, want nil", *got.LinkedDocumentID)
	}
	if got.LinkCreator != nil {
		t.Errorf("LinkCreator = %v, want nil", *got.LinkCreator)
	}
	if got.LinkConflicting {
		t.Error("LinkConflicting = true, want false")
	}

	// doc can be linked to another movement
	if err := e.svc.Link(ctx, mv2.ID, docID, "manual"); err != nil {
		t.Fatalf("re-Link to doc failed: %v", err)
	}
	got2, _ := e.svc.GetMovement(ctx, mv2.ID)
	if got2.LinkedDocumentID == nil || *got2.LinkedDocumentID != docID {
		t.Errorf("mv2 LinkedDocumentID = %v, want %q", got2.LinkedDocumentID, docID)
	}
}

// TestUnlinkIdempotentNoOp — unlinking an unlinked movement is nil.
func TestUnlinkIdempotentNoOp(t *testing.T) {
	t.Parallel()
	e := newTestEnv()
	ctx := ctxWithTenant(testTenant)
	a := e.createAccount(t, ctx, "A", entity.AccountTypeBank, "INR")

	mv, _ := e.svc.CreateManualMovement(ctx, MovementInput{
		Kind: entity.KindExpense, Amount: "100", Currency: "INR",
		OccurredOn:  time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		Description: "test", SourceAccountID: a,
	})
	before, _ := e.movements.get(mv.ID)

	if err := e.svc.Unlink(ctx, mv.ID); err != nil {
		t.Fatalf("Unlink on unlinked movement returned %v, want nil", err)
	}
	after, _ := e.movements.get(mv.ID)
	if !reflect.DeepEqual(before, after) {
		t.Error("movement changed after no-op Unlink")
	}
}

// TestLinkConflictRetentionDisagreeing — LinkConflicting=true when doc
// fields disagree with movement amount/currency.
func TestLinkConflictRetentionDisagreeing(t *testing.T) {
	t.Parallel()
	e := newTestEnv()
	ctx := ctxWithTenant(testTenant)
	a := e.createAccount(t, ctx, "A", entity.AccountTypeBank, "INR")

	mv, _ := e.svc.CreateManualMovement(ctx, MovementInput{
		Kind: entity.KindExpense, Amount: "40000", Currency: "INR",
		OccurredOn:  time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		Description: "test", SourceAccountID: a,
	})
	docID := e.createDoc(t, ctx, map[string]any{
		"price":    "39500",
		"currency": "INR",
	})

	if err := e.svc.Link(ctx, mv.ID, docID, "manual"); err != nil {
		t.Fatalf("Link failed: %v", err)
	}

	got, _ := e.svc.GetMovement(ctx, mv.ID)
	if !got.LinkConflicting {
		t.Error("LinkConflicting = false, want true")
	}
	if got.Amount != "40000" {
		t.Errorf("Amount = %q, want %q (unchanged)", got.Amount, "40000")
	}

	doc, _ := e.docs.get(docID)
	if doc.ExtractedFields["price"] != "39500" {
		t.Errorf("doc price = %v, want %q", doc.ExtractedFields["price"], "39500")
	}
}

// TestLinkAgreeingNoConflict — LinkConflicting=false when fields agree
// or when price/currency is missing.
func TestLinkAgreeingNoConflict(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		fields map[string]any
	}{
		{"agreeing", map[string]any{"price": "40000", "currency": "INR"}},
		{"price only", map[string]any{"price": "40000"}},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := newTestEnv()
			ctx := ctxWithTenant(testTenant)
			a := e.createAccount(t, ctx, "A", entity.AccountTypeBank, "INR")

			mv, _ := e.svc.CreateManualMovement(ctx, MovementInput{
				Kind: entity.KindExpense, Amount: "40000", Currency: "INR",
				OccurredOn:  time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
				Description: "test", SourceAccountID: a,
			})
			docID := e.createDoc(t, ctx, tc.fields)

			if err := e.svc.Link(ctx, mv.ID, docID, "manual"); err != nil {
				t.Fatalf("Link failed: %v", err)
			}

			got, _ := e.svc.GetMovement(ctx, mv.ID)
			if got.LinkConflicting {
				t.Error("LinkConflicting = true, want false")
			}
		})
	}
}

// TestMovementCoreFieldsImmutable — Link + Unlink + PatchDescription do not
// change core fields.
func TestMovementCoreFieldsImmutable(t *testing.T) {
	t.Parallel()
	e := newTestEnv()
	ctx := ctxWithTenant(testTenant)
	a := e.createAccount(t, ctx, "A", entity.AccountTypeBank, "INR")

	mv, _ := e.svc.CreateManualMovement(ctx, MovementInput{
		Kind: entity.KindExpense, Amount: "100", Currency: "INR",
		OccurredOn:  time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		Description: "original", SourceAccountID: a,
	})
	before := coreOf(mv)
	docID := e.createDoc(t, ctx, nil)

	if err := e.svc.Link(ctx, mv.ID, docID, "manual"); err != nil {
		t.Fatalf("Link failed: %v", err)
	}
	mid1, _ := e.svc.GetMovement(ctx, mv.ID)
	assertCoreUnchanged(t, "after link", before, coreOf(mid1))

	if err := e.svc.Unlink(ctx, mv.ID); err != nil {
		t.Fatalf("Unlink failed: %v", err)
	}
	mid2, _ := e.svc.GetMovement(ctx, mv.ID)
	assertCoreUnchanged(t, "after unlink", before, coreOf(mid2))

	if _, err := e.svc.PatchDescription(ctx, mv.ID, "updated"); err != nil {
		t.Fatalf("PatchDescription failed: %v", err)
	}
	mid3, _ := e.svc.GetMovement(ctx, mv.ID)
	assertCoreUnchanged(t, "after patch", before, coreOf(mid3))
}

// TestPatchDescriptionSucceeds — patch changes only description + norm.
func TestPatchDescriptionSucceeds(t *testing.T) {
	t.Parallel()

	// helper: run the patch scenario on a movement with the given origin
	run := func(t *testing.T, origin string) {
		t.Helper()
		e := newTestEnv()
		ctx := ctxWithTenant(testTenant)
		a := e.createAccount(t, ctx, "A", entity.AccountTypeBank, "INR")

		var mv entity.MoneyMovement
		var err error
		if origin == entity.OriginManual {
			mv, err = e.svc.CreateManualMovement(ctx, MovementInput{
				Kind: entity.KindExpense, Amount: "100", Currency: "INR",
				OccurredOn:  time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
				Description: "REL DIG 123", SourceAccountID: a,
			})
			if err != nil {
				t.Fatalf("CreateManualMovement failed: %v", err)
			}
		} else {
			mv.ID = e.seedMovement(t, entity.MoneyMovement{
				TenantID:        testTenant,
				Kind:            entity.KindExpense,
				Amount:          "100",
				Currency:        "INR",
				OccurredOn:      time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
				Description:     "REL DIG 123",
				NormDescription: "rel dig 123",
				Origin:          entity.OriginImport,
				SourceAccountID: strPtr(a),
			})
		}

		before, _ := e.movements.get(mv.ID)

		patched, err := e.svc.PatchDescription(ctx, mv.ID, "Reliance Digital")
		if err != nil {
			t.Fatalf("PatchDescription failed: %v", err)
		}

		// returned value
		if patched.Description != "Reliance Digital" {
			t.Errorf("returned Description = %q, want %q", patched.Description, "Reliance Digital")
		}
		if patched.NormDescription != "reliance digital" {
			t.Errorf("returned NormDescription = %q, want %q", patched.NormDescription, "reliance digital")
		}

		// re-fetched
		got, err := e.svc.GetMovement(ctx, mv.ID)
		if err != nil {
			t.Fatalf("GetMovement failed: %v", err)
		}
		if got.Description != "Reliance Digital" {
			t.Errorf("fetched Description = %q, want %q", got.Description, "Reliance Digital")
		}
		if got.NormDescription != "reliance digital" {
			t.Errorf("fetched NormDescription = %q, want %q", got.NormDescription, "reliance digital")
		}

		// all other fields byte-identical
		assertOtherFieldsUnchanged(t, before, got)
	}

	t.Run("manual", func(t *testing.T) {
		t.Parallel()
		run(t, entity.OriginManual)
	})
	t.Run("import", func(t *testing.T) {
		t.Parallel()
		run(t, entity.OriginImport)
	})
}

// assertOtherFieldsUnchanged checks every field except Description and
// NormDescription is byte-identical.
func assertOtherFieldsUnchanged(t *testing.T, before, after entity.MoneyMovement) {
	t.Helper()
	if before.ID != after.ID {
		t.Errorf("ID changed: %q -> %q", before.ID, after.ID)
	}
	if before.TenantID != after.TenantID {
		t.Errorf("TenantID changed: %q -> %q", before.TenantID, after.TenantID)
	}
	if before.Kind != after.Kind {
		t.Errorf("Kind changed: %q -> %q", before.Kind, after.Kind)
	}
	if before.Amount != after.Amount {
		t.Errorf("Amount changed: %q -> %q", before.Amount, after.Amount)
	}
	if before.Currency != after.Currency {
		t.Errorf("Currency changed: %q -> %q", before.Currency, after.Currency)
	}
	if !before.OccurredOn.Equal(after.OccurredOn) {
		t.Errorf("OccurredOn changed: %v -> %v", before.OccurredOn, after.OccurredOn)
	}
	if !before.RecordedAt.Equal(after.RecordedAt) {
		t.Errorf("RecordedAt changed: %v -> %v", before.RecordedAt, after.RecordedAt)
	}
	if before.Origin != after.Origin {
		t.Errorf("Origin changed: %q -> %q", before.Origin, after.Origin)
	}
	if !ptrEq(before.SourceAccountID, after.SourceAccountID) {
		t.Errorf("SourceAccountID changed")
	}
	if !ptrEq(before.DestinationAccountID, after.DestinationAccountID) {
		t.Errorf("DestinationAccountID changed")
	}
	if !ptrEq(before.ImportBatchID, after.ImportBatchID) {
		t.Errorf("ImportBatchID changed")
	}
	if before.ImportLine != after.ImportLine {
		t.Errorf("ImportLine changed: %v -> %v", before.ImportLine, after.ImportLine)
	}
	if !ptrEq(before.ExternalReference, after.ExternalReference) {
		t.Errorf("ExternalReference changed")
	}
	if !ptrEq(before.LinkedDocumentID, after.LinkedDocumentID) {
		t.Errorf("LinkedDocumentID changed")
	}
	if !ptrEq(before.LinkCreator, after.LinkCreator) {
		t.Errorf("LinkCreator changed")
	}
	if before.LinkConflicting != after.LinkConflicting {
		t.Errorf("LinkConflicting changed: %v -> %v", before.LinkConflicting, after.LinkConflicting)
	}
	if !before.CreatedAt.Equal(after.CreatedAt) {
		t.Errorf("CreatedAt changed: %v -> %v", before.CreatedAt, after.CreatedAt)
	}
}

// TestPatchDescriptionBlankRejected — blank description is ErrInvalid,
// no repo call.
func TestPatchDescriptionBlankRejected(t *testing.T) {
	t.Parallel()

	for _, desc := range []string{"", "   "} {
		desc := desc
		t.Run("desc="+strconv.Quote(desc), func(t *testing.T) {
			t.Parallel()
			e := newTestEnv()
			ctx := ctxWithTenant(testTenant)
			a := e.createAccount(t, ctx, "A", entity.AccountTypeBank, "INR")
			mv, _ := e.svc.CreateManualMovement(ctx, MovementInput{
				Kind: entity.KindExpense, Amount: "100", Currency: "INR",
				OccurredOn:  time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
				Description: "original", SourceAccountID: a,
			})

			callsBefore := e.totalCalls()
			_, err := e.svc.PatchDescription(ctx, mv.ID, desc)
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("PatchDescription(%q) returned %v, want ErrInvalid", desc, err)
			}
			if got := e.totalCalls(); got != callsBefore {
				t.Fatalf("repo calls changed: %d -> %d, want 0 new calls", callsBefore, got)
			}

			got, _ := e.svc.GetMovement(ctx, mv.ID)
			if got.Description != "original" {
				t.Errorf("Description = %q, want %q (unchanged)", got.Description, "original")
			}
		})
	}
}

// TestDeleteManualMovement — manual movement can be deleted.
func TestDeleteManualMovement(t *testing.T) {
	t.Parallel()
	e := newTestEnv()
	ctx := ctxWithTenant(testTenant)
	a := e.createAccount(t, ctx, "A", entity.AccountTypeBank, "INR")

	mv, _ := e.svc.CreateManualMovement(ctx, MovementInput{
		Kind: entity.KindExpense, Amount: "100", Currency: "INR",
		OccurredOn:  time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		Description: "test", SourceAccountID: a,
	})

	if err := e.svc.DeleteMovement(ctx, mv.ID); err != nil {
		t.Fatalf("DeleteMovement failed: %v", err)
	}
	if _, err := e.svc.GetMovement(ctx, mv.ID); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("GetMovement after delete returned %v, want ErrNotFound", err)
	}
	if bal := e.balanceOf(t, ctx, a); bal != "0" {
		t.Fatalf("balance after delete = %q, want %q", bal, "0")
	}
}

// TestDeleteUnknownMovementNotFound — deleting a nonexistent movement.
func TestDeleteUnknownMovementNotFound(t *testing.T) {
	t.Parallel()
	e := newTestEnv()
	ctx := ctxWithTenant(testTenant)
	if err := e.svc.DeleteMovement(ctx, "ghost"); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("DeleteMovement(ghost) returned %v, want ErrNotFound", err)
	}
}

// TestDeleteImportedMovementConflict — import-origin movements cannot be
// deleted.
func TestDeleteImportedMovementConflict(t *testing.T) {
	t.Parallel()
	e := newTestEnv()
	ctx := ctxWithTenant(testTenant)
	a := e.createAccount(t, ctx, "A", entity.AccountTypeBank, "INR")

	line := 3
	importedID := e.seedMovement(t, entity.MoneyMovement{
		TenantID:          testTenant,
		Kind:              entity.KindExpense,
		Amount:            "500",
		Currency:          "INR",
		OccurredOn:        time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		Description:       "imported",
		NormDescription:   "imported",
		Origin:            entity.OriginImport,
		SourceAccountID:   strPtr(a),
		ImportBatchID:     strPtr("batch-1"),
		ImportLine:        &line,
		ExternalReference: strPtr("EXT-9"),
	})

	before, _ := e.movements.get(importedID)

	err := e.svc.DeleteMovement(ctx, importedID)
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("DeleteMovement(imported) returned %v, want ErrConflict", err)
	}

	after, ok := e.movements.get(importedID)
	if !ok {
		t.Fatal("imported movement was deleted, want unchanged")
	}
	if !reflect.DeepEqual(before, after) {
		t.Error("imported movement changed")
	}
}

// TestTenantSameNameTwoTenants — same account name in two tenants.
func TestTenantSameNameTwoTenants(t *testing.T) {
	t.Parallel()
	e := newTestEnv()
	ctxA := ctxWithTenant(testTenant)
	ctxB := ctxWithTenant(testTenantB)

	idA := e.createAccount(t, ctxA, "Cash", entity.AccountTypeCash, "INR")
	idB := e.createAccount(t, ctxB, "Cash", entity.AccountTypeCash, "INR")
	if idA == idB {
		t.Fatalf("same ID %q for both tenants", idA)
	}

	listA, err := e.svc.ListAccounts(ctxA)
	if err != nil {
		t.Fatalf("ListAccounts(A) failed: %v", err)
	}
	if len(listA) != 1 || listA[0].ID != idA {
		t.Fatalf("ListAccounts(A) = %+v, want exactly [%q]", listA, idA)
	}

	listB, err := e.svc.ListAccounts(ctxB)
	if err != nil {
		t.Fatalf("ListAccounts(B) failed: %v", err)
	}
	if len(listB) != 1 || listB[0].ID != idB {
		t.Fatalf("ListAccounts(B) = %+v, want exactly [%q]", listB, idB)
	}
}

// TestTenantForeignIDNotFound — foreign-tenant IDs are not found.
func TestTenantForeignIDNotFound(t *testing.T) {
	t.Parallel()
	e := newTestEnv()
	ctxA := ctxWithTenant(testTenant)
	ctxB := ctxWithTenant(testTenantB)

	accB := e.createAccount(t, ctxB, "B-acc", entity.AccountTypeBank, "INR")
	mvB, _ := e.svc.CreateManualMovement(ctxB, MovementInput{
		Kind: entity.KindExpense, Amount: "100", Currency: "INR",
		OccurredOn:  time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		Description: "b-mv", SourceAccountID: accB,
	})

	// GetAccount(B's account) from A → ErrNotFound
	if _, _, err := e.svc.GetAccount(ctxA, accB); !errors.Is(err, repo.ErrNotFound) {
		t.Errorf("GetAccount(B acc) from A returned %v, want ErrNotFound", err)
	}
	// GetMovement(B's movement) from A → ErrNotFound
	if _, err := e.svc.GetMovement(ctxA, mvB.ID); !errors.Is(err, repo.ErrNotFound) {
		t.Errorf("GetMovement(B mv) from A returned %v, want ErrNotFound", err)
	}
	// PatchDescription(B's movement) from A → ErrNotFound
	if _, err := e.svc.PatchDescription(ctxA, mvB.ID, "hack"); !errors.Is(err, repo.ErrNotFound) {
		t.Errorf("PatchDescription(B mv) from A returned %v, want ErrNotFound", err)
	}
	// DeleteMovement(B's manual movement) from A → ErrNotFound
	if err := e.svc.DeleteMovement(ctxA, mvB.ID); !errors.Is(err, repo.ErrNotFound) {
		t.Errorf("DeleteMovement(B mv) from A returned %v, want ErrNotFound", err)
	}

	// B's row still present
	if _, ok := e.movements.get(mvB.ID); !ok {
		t.Fatal("B's movement was deleted by A, want unchanged")
	}
	if _, ok := e.accounts.get(accB); !ok {
		t.Fatal("B's account was deleted by A, want unchanged")
	}
}

// TestNoTenantFailsClosed — no tenant in ctx → ErrNoTenant, zero repo calls.
func TestNoTenantFailsClosed(t *testing.T) {
	t.Parallel()
	e := newTestEnv()
	noTenantCtx := context.Background()

	a := e.createAccount(t, ctxWithTenant(testTenant), "A", entity.AccountTypeBank, "INR")
	mv, _ := e.svc.CreateManualMovement(ctxWithTenant(testTenant), MovementInput{
		Kind: entity.KindExpense, Amount: "100", Currency: "INR",
		OccurredOn:  time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		Description: "test", SourceAccountID: a,
	})
	docID := e.createDoc(t, ctxWithTenant(testTenant), nil)

	calls := []struct {
		name string
		fn   func() error
	}{
		{"CreateAccount", func() error {
			_, err := e.svc.CreateAccount(noTenantCtx, AccountInput{Name: "X", Type: "bank", Currency: "INR"})
			return err
		}},
		{"ListAccounts", func() error {
			_, err := e.svc.ListAccounts(noTenantCtx)
			return err
		}},
		{"GetAccount", func() error {
			_, _, err := e.svc.GetAccount(noTenantCtx, a)
			return err
		}},
		{"CreateManualMovement", func() error {
			_, err := e.svc.CreateManualMovement(noTenantCtx, MovementInput{
				Kind: entity.KindExpense, Amount: "1", Currency: "INR",
				OccurredOn: time.Now(), Description: "x", SourceAccountID: a,
			})
			return err
		}},
		{"ListMovements", func() error {
			_, err := e.svc.ListMovements(noTenantCtx, MovementListFilter{})
			return err
		}},
		{"GetMovement", func() error {
			_, err := e.svc.GetMovement(noTenantCtx, mv.ID)
			return err
		}},
		{"PatchDescription", func() error {
			_, err := e.svc.PatchDescription(noTenantCtx, mv.ID, "x")
			return err
		}},
		{"DeleteMovement", func() error {
			return e.svc.DeleteMovement(noTenantCtx, mv.ID)
		}},
		{"Link", func() error {
			return e.svc.Link(noTenantCtx, mv.ID, docID, "manual")
		}},
		{"Unlink", func() error {
			return e.svc.Unlink(noTenantCtx, mv.ID)
		}},
	}

	for _, tc := range calls {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			before := e.totalCalls()
			err := tc.fn()
			if !errors.Is(err, tenant.ErrNoTenant) {
				t.Fatalf("%s returned %v, want ErrNoTenant", tc.name, err)
			}
			if got := e.totalCalls(); got != before {
				t.Fatalf("%s: repo calls changed %d -> %d, want 0 new", tc.name, before, got)
			}
		})
	}
}

// TestGetAccountUnknown — GetAccount with unknown ID.
func TestGetAccountUnknown(t *testing.T) {
	t.Parallel()
	e := newTestEnv()
	ctx := ctxWithTenant(testTenant)
	if _, _, err := e.svc.GetAccount(ctx, "ghost"); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("GetAccount(ghost) returned %v, want ErrNotFound", err)
	}
}
