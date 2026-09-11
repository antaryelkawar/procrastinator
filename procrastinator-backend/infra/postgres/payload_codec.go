package postgres

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"procrastinator-backend/commons"
	"procrastinator-backend/commons/entity"
)

// payloadCodec marshals/unmarshals an entity's data portion to/from the
// payload column's data.* JSON object and owns the per-kind dynamic-query
// whitelist (filters). The generic engine consumes this API: table +
// selectCols give the physical row shape, columns() gives the non-payload
// INSERT/UPDATE values, marshal/unmarshal give the payload.data.* object
// (plus clearing keys for kinds that support partial-update clearing
// semantics), and filters gives the filter/order whitelist for validation.
//
// Money values (Amount, Price, import-line Amount — exact-decimal Go strings)
// are carried in the data map as json.Number so they serialize to JSON numbers
// and unmarshal back to the exact decimal string, never via float64. Dates
// are "2006-01-02" strings; timestamps are RFC3339 strings; zero times are
// absent. Absent keys unmarshal to zero values, and unknown keys are ignored
// (forward compatibility).
type payloadCodec[T any] struct {
	// table is the SQL table name for this kind.
	table string
	// selectCols is the explicit SELECT column list (also the scan order).
	selectCols []string
	// columns extracts the real (non-payload) column values to INSERT/UPDATE,
	// excluding id and owner_id (the engine adds those). Same non-zero/nil
	// inclusion rules the old toMap functions used.
	columns func(T) map[string]any
	// marshal extracts the data.* fields. Returns the upsert map (only
	// non-zero/non-nil fields, per the per-kind rules) and, for kinds with
	// clearing semantics, the keys to explicitly remove.
	marshal func(T) (data map[string]any, clear []string)
	// unmarshal fills the data portion of ent from the data.* object.
	unmarshal func(data map[string]any, ent *T) error
	// filters is the entity's filter/order whitelist: the filterable field
	// names (fieldCols) and orderable field names (orderCols) the generic
	// engine validates caller input against. Declared per kind here (design D3)
	// so the codec is the single source of truth for the dynamic-query
	// whitelists; the engine reads it via codec.filters().
	filters filterConfig
}

// filterConfig returns the codec's filter/order whitelist (design D3: the
// codec owns the per-kind whitelist, so the engine reads codec.filters).
func (c *payloadCodec[T]) filterConfig() filterConfig { return c.filters }

// ---------------------------------------------------------------------------
// Shared data-map helpers
// ---------------------------------------------------------------------------

// dataStr sets data[key] = *p for a non-nil *string.
func dataStr(data map[string]any, key string, p *string) {
	if p != nil {
		data[key] = *p
	}
}

// dataStrNE sets data[key] = s for a non-empty string.
func dataStrNE(data map[string]any, key, s string) {
	if s != "" {
		data[key] = s
	}
}

// dataMoney sets data[key] = json.Number(s) for a non-empty exact-decimal
// string. json.Number keeps the value a JSON number while preserving the
// exact decimal (never a float64).
func dataMoney(data map[string]any, key, s string) {
	if s != "" {
		data[key] = json.Number(s)
	}
}

// dataDate sets data[key] = "2006-01-02" for a non-zero date.
func dataDate(data map[string]any, key string, t time.Time) {
	if !t.IsZero() {
		data[key] = t.Format("2006-01-02")
	}
}

// dataDatePtr is dataDate for *time.Time.
func dataDatePtr(data map[string]any, key string, t *time.Time) {
	if t != nil {
		data[key] = t.Format("2006-01-02")
	}
}

// dataTS sets data[key] = RFC3339 for a non-zero timestamp.
func dataTS(data map[string]any, key string, t time.Time) {
	if !t.IsZero() {
		data[key] = t.Format(time.RFC3339)
	}
}

// dataTSPtr is dataTS for *time.Time.
func dataTSPtr(data map[string]any, key string, t *time.Time) {
	if t != nil {
		data[key] = t.Format(time.RFC3339)
	}
}

// dataInt64 sets data[key] for a positive int64.
func dataInt64(data map[string]any, key string, v int64) {
	if v > 0 {
		data[key] = v
	}
}

// dataInt sets data[key] for a positive int.
func dataInt(data map[string]any, key string, v int) {
	if v > 0 {
		data[key] = v
	}
}

// dataIntPtr sets data[key] = json.Number for a non-nil *int (JSON number).
func dataIntPtr(data map[string]any, key string, v *int) {
	if v != nil {
		data[key] = json.Number(fmt.Sprintf("%d", *v))
	}
}

// dataBool sets data[key] = true for a true value (include-when-true rule).
func dataBool(data map[string]any, key string, v bool) {
	if v {
		data[key] = true
	}
}

// dataMap sets data[key] = m for a non-nil map (JSON object).
func dataMap(data map[string]any, key string, m map[string]any) {
	if m != nil {
		data[key] = m
	}
}

// dataFloat64 sets data[key] = *p for a non-nil *float64 (JSON number).
func dataFloat64(data map[string]any, key string, p *float64) {
	if p != nil {
		data[key] = *p
	}
}

// ---------------------------------------------------------------------------
// Shared unmarshal helpers (all no-ops on absent or nil values)
// ---------------------------------------------------------------------------

// unStr fills *p from a JSON string.
func unStr(data map[string]any, key string, p **string) {
	if v, ok := data[key].(string); ok {
		s := v
		*p = &s
	}
}

// numString returns the exact decimal string for a numeric data value. A
// decoded payload yields json.Number (exact); a directly-marshaled data map
// yields native Go numbers, converted without float64 rounding for integers.
func numString(v any) (string, bool) {
	switch n := v.(type) {
	case json.Number:
		return n.String(), true
	case float64:
		return strconv.FormatFloat(n, 'f', -1, 64), true
	case int64:
		return strconv.FormatInt(n, 10), true
	case int:
		return strconv.Itoa(n), true
	}
	return "", false
}

// numFloat64 converts a numeric data value (json.Number or native Go number).
func numFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	case float64:
		return n, true
	case int64:
		return float64(n), true
	case int:
		return float64(n), true
	}
	return 0, false
}

// numInt64 converts a numeric data value (json.Number or native Go number).
func numInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case json.Number:
		i, err := n.Int64()
		return i, err == nil
	case float64:
		return int64(n), true
	case int64:
		return n, true
	case int:
		return int64(n), true
	}
	return 0, false
}

// unMoney fills *p from a JSON number (exact decimal string, never float64).
func unMoney(data map[string]any, key string, p **string) {
	if v, ok := data[key]; ok {
		if s, ok2 := numString(v); ok2 {
			*p = &s
		}
	}
}

// unMoneyVal fills *p (string field) from a JSON number.
func unMoneyVal(data map[string]any, key string, p *string) {
	if v, ok := data[key]; ok {
		if s, ok2 := numString(v); ok2 {
			*p = s
		}
	}
}

// unDate fills *p from a "2006-01-02" string (UTC).
func unDate(data map[string]any, key string, p **time.Time) {
	if v, ok := data[key].(string); ok {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			*p = &t
		}
	}
}

// unDateVal is unDate for value time.Time fields.
func unDateVal(data map[string]any, key string, p *time.Time) {
	if v, ok := data[key].(string); ok {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			*p = t
		}
	}
}

// unTS fills *p from an RFC3339 string.
func unTS(data map[string]any, key string, p **time.Time) {
	if v, ok := data[key].(string); ok {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			*p = &t
		}
	}
}

// unTSVal is unTS for value time.Time fields.
func unTSVal(data map[string]any, key string, p *time.Time) {
	if v, ok := data[key].(string); ok {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			*p = t
		}
	}
}

// unInt64 fills *p from a JSON number.
func unInt64(data map[string]any, key string, p *int64) {
	if v, ok := data[key]; ok {
		if n, ok2 := numInt64(v); ok2 {
			*p = n
		}
	}
}

// unInt fills *p from a JSON number.
func unInt(data map[string]any, key string, p **int) {
	if v, ok := data[key]; ok {
		if n, ok2 := numInt64(v); ok2 {
			i := int(n)
			*p = &i
		}
	}
}

// unBool fills *p from a JSON boolean (absent → false).
func unBool(data map[string]any, key string, p *bool) {
	if v, ok := data[key].(bool); ok {
		*p = v
	}
}

// unMap fills *p from a JSON object. The value may be a map[string]any (from
// a hand-built data map) or the object form produced by decoding the payload
// JSON (the engine decodes via UseNumber, which yields map[string]any for
// objects); both forms are accepted.
func unMap(data map[string]any, key string, p *map[string]any) {
	if v, ok := data[key].(map[string]any); ok {
		*p = v
	}
}

// unFloat64 fills *p from a JSON number.
func unFloat64(data map[string]any, key string, p **float64) {
	if v, ok := data[key]; ok {
		if f, ok2 := numFloat64(v); ok2 {
			*p = &f
		}
	}
}

// normDescriptionOf derives norm_description from description, preserving the
// old toMap rule: use the provided norm when non-empty, else normalize.
func normDescriptionOf(norm, desc string) string {
	if norm != "" {
		return norm
	}
	return commons.NormalizeDescription(desc)
}

// ---------------------------------------------------------------------------
// Per-kind filter/order whitelists (design D3)
// ---------------------------------------------------------------------------
// Each codec below declares its own filter/order whitelist: fieldCols maps
// filterable field names to SQL expressions for WHERE clauses (bare column
// names or payload data expressions), orderCols maps orderable field names to
// SQL expressions for ORDER BY clauses. Every caller-influenced name is
// validated against these maps before SQL assembly; unknown names are
// rejected, never interpolated.

var assetFieldCols = map[string]string{
	"id":                 "id",
	"brand":              "(payload #>> '{data,brand}')",
	"model":              "(payload #>> '{data,model}')",
	"serial_number":      "(payload #>> '{data,serial}')",
	"norm_serial":        "(payload #>> '{data,norm_serial}')",
	"norm_brand":         "(payload #>> '{data,norm_brand}')",
	"norm_model":         "(payload #>> '{data,norm_model}')",
	"name":               "(payload #>> '{data,name}')",
	"norm_name":          "(payload #>> '{data,norm_name}')",
	"asset_category":     "(payload #>> '{data,asset_category}')",
	"purchase_date":      "((payload #>> '{data,purchase_date}')::date)",
	"warranty_end":       "((payload #>> '{data,warranty_end}')::date)",
	"price":              "((payload #>> '{data,price}')::numeric)",
	"currency":           "(payload #>> '{data,currency}')",
	"created_at":         "created_at",
	"updated_at":         "updated_at",
	"deleted_at":         "deleted_at",
	"owner_household_id": "owner_household_id",
}

var assetOrderCols = map[string]string{
	"id":            "id",
	"created_at":    "created_at",
	"updated_at":    "updated_at",
	"brand":         "(payload #>> '{data,brand}')",
	"model":         "(payload #>> '{data,model}')",
	"name":          "(payload #>> '{data,name}')",
	"purchase_date": "((payload #>> '{data,purchase_date}')::date)",
	"warranty_end":  "((payload #>> '{data,warranty_end}')::date)",
}

// ---------------------------------------------------------------------------
// Per-kind codecs
// ---------------------------------------------------------------------------

var assetCodec = &payloadCodec[entity.Asset]{
	table:      "assets",
	selectCols: []string{"id", "owner_id", "owner_household_id", "deleted_at", "created_at", "updated_at", "payload"},
	filters:    filterConfig{fieldCols: assetFieldCols, orderCols: assetOrderCols},
	columns: func(a entity.Asset) map[string]any {
		m := make(map[string]any)
		if a.OwnerHouseholdID != nil {
			m["owner_household_id"] = a.OwnerHouseholdID
		}
		if a.DeletedAt != nil {
			m["deleted_at"] = a.DeletedAt
		}
		return m
	},
	marshal: func(a entity.Asset) (map[string]any, []string) {
		m := make(map[string]any)
		// Raw identity fields + norm derivation, preserved from the old
		// assetToMap: the raw field is always emitted when non-nil, and the
		// norm is derived (commons.NormalizeName/NormalizeSerial) when the
		// norm is nil, or included as-is when the norm is set (even with the
		// raw field nil).
		dataStr(m, "name", a.Name)
		dataStr(m, "brand", a.Brand)
		dataStr(m, "model", a.Model)
		dataStr(m, "serial", a.SerialNumber)
		if a.Brand != nil {
			if a.NormBrand != nil {
				m["norm_brand"] = *a.NormBrand
			} else {
				nb := commons.NormalizeName(*a.Brand)
				m["norm_brand"] = nb
			}
		} else if a.NormBrand != nil {
			m["norm_brand"] = *a.NormBrand
		}
		if a.Model != nil {
			if a.NormModel != nil {
				m["norm_model"] = *a.NormModel
			} else {
				nm := commons.NormalizeName(*a.Model)
				m["norm_model"] = nm
			}
		} else if a.NormModel != nil {
			m["norm_model"] = *a.NormModel
		}
		if a.SerialNumber != nil {
			if a.NormSerial != nil {
				m["norm_serial"] = *a.NormSerial
			} else {
				ns := commons.NormalizeSerial(*a.SerialNumber)
				m["norm_serial"] = ns
			}
		} else if a.NormSerial != nil {
			m["norm_serial"] = *a.NormSerial
		}
		if a.Name != nil {
			if a.NormName != nil {
				m["norm_name"] = *a.NormName
			} else {
				nn := commons.NormalizeName(*a.Name)
				m["norm_name"] = nn
			}
		} else if a.NormName != nil {
			m["norm_name"] = *a.NormName
		}
		dataDatePtr(m, "purchase_date", a.PurchaseDate)
		dataDatePtr(m, "warranty_end", a.WarrantyEnd)
		dataMoney(m, "price", strDeref(a.Price))
		dataStr(m, "currency", a.Currency)
		dataMap(m, "metadata", a.Metadata)
		dataFloat64(m, "confidence", a.Confidence)
		dataStr(m, "asset_category", a.AssetCategory)
		dataFloat64(m, "category_confidence", a.CategoryConfidence)
		dataBool(m, "category_user_set", a.CategoryUserSet)
		dataStr(m, "merged_into", a.MergedInto)
		dataTSPtr(m, "merged_at", a.MergedAt)
		return m, nil
	},
	unmarshal: func(data map[string]any, a *entity.Asset) error {
		unStr(data, "name", &a.Name)
		unStr(data, "brand", &a.Brand)
		unStr(data, "model", &a.Model)
		unStr(data, "serial", &a.SerialNumber)
		unStr(data, "norm_name", &a.NormName)
		unStr(data, "norm_brand", &a.NormBrand)
		unStr(data, "norm_model", &a.NormModel)
		unStr(data, "norm_serial", &a.NormSerial)
		unDate(data, "purchase_date", &a.PurchaseDate)
		unDate(data, "warranty_end", &a.WarrantyEnd)
		unMoney(data, "price", &a.Price)
		unStr(data, "currency", &a.Currency)
		unMap(data, "metadata", &a.Metadata)
		unFloat64(data, "confidence", &a.Confidence)
		unStr(data, "asset_category", &a.AssetCategory)
		unFloat64(data, "category_confidence", &a.CategoryConfidence)
		unBool(data, "category_user_set", &a.CategoryUserSet)
		unStr(data, "merged_into", &a.MergedInto)
		unTS(data, "merged_at", &a.MergedAt)
		return nil
	},
}

var sourceFieldCols = map[string]string{
	"id":                 "id",
	"filename":           "(payload #>> '{data,filename}')",
	"content_type":       "(payload #>> '{data,content_type}')",
	"byte_size":          "((payload #>> '{data,byte_size}')::bigint)",
	"sha256":             "(payload #>> '{data,sha256}')",
	"uploaded_at":        "((payload #>> '{data,uploaded_at}')::timestamptz)",
	"owner_household_id": "owner_household_id",
}

var sourceOrderCols = map[string]string{
	"id":          "id",
	"uploaded_at": "((payload #>> '{data,uploaded_at}')::timestamptz)",
	"byte_size":   "((payload #>> '{data,byte_size}')::bigint)",
}

var sourceCodec = &payloadCodec[entity.Source]{
	table:      "sources",
	selectCols: []string{"id", "owner_id", "owner_household_id", "deleted_at", "created_at", "updated_at", "payload"},
	filters:    filterConfig{fieldCols: sourceFieldCols, orderCols: sourceOrderCols},
	columns: func(s entity.Source) map[string]any {
		m := make(map[string]any)
		if s.OwnerHouseholdID != nil {
			m["owner_household_id"] = s.OwnerHouseholdID
		}
		return m
	},
	marshal: func(s entity.Source) (map[string]any, []string) {
		m := make(map[string]any)
		dataStrNE(m, "filename", s.Filename)
		dataStrNE(m, "content_type", s.ContentType)
		dataInt64(m, "byte_size", s.Size)
		dataStrNE(m, "storage_path", s.Path)
		dataStrNE(m, "sha256", s.SHA256)
		dataTS(m, "uploaded_at", s.UploadedAt)
		return m, nil
	},
	unmarshal: func(data map[string]any, s *entity.Source) error {
		if v, ok := data["filename"].(string); ok {
			s.Filename = v
		}
		if v, ok := data["content_type"].(string); ok {
			s.ContentType = v
		}
		unInt64(data, "byte_size", &s.Size)
		if v, ok := data["storage_path"].(string); ok {
			s.Path = v
		}
		if v, ok := data["sha256"].(string); ok {
			s.SHA256 = v
		}
		unTSVal(data, "uploaded_at", &s.UploadedAt)
		return nil
	},
}

var documentFieldCols = map[string]string{
	"id":                 "id",
	"asset_id":           "asset_id",
	"source_id":          "source_id",
	"doc_type":           "(payload #>> '{data,doc_type}')",
	"created_at":         "created_at",
	"owner_household_id": "owner_household_id",
}

var documentOrderCols = map[string]string{
	"id":         "id",
	"created_at": "created_at",
	"doc_type":   "(payload #>> '{data,doc_type}')",
}

var documentCodec = &payloadCodec[entity.Document]{
	table:      "documents",
	selectCols: []string{"id", "owner_id", "owner_household_id", "asset_id", "source_id", "deleted_at", "created_at", "updated_at", "payload"},
	filters:    filterConfig{fieldCols: documentFieldCols, orderCols: documentOrderCols},
	columns: func(d entity.Document) map[string]any {
		m := make(map[string]any)
		if d.OwnerHouseholdID != nil {
			m["owner_household_id"] = d.OwnerHouseholdID
		}
		if d.AssetID != "" {
			m["asset_id"] = d.AssetID
		}
		if d.SourceID != "" {
			m["source_id"] = d.SourceID
		}
		// Soft-delete: persist deleted_at when set (mirrors the asset codec).
		// Without this the documents delete handler's `doc.DeletedAt = &now`
		// was silently dropped by the codec and the row was never hidden.
		if d.DeletedAt != nil {
			m["deleted_at"] = d.DeletedAt
		}
		return m
	},
	marshal: func(d entity.Document) (map[string]any, []string) {
		m := make(map[string]any)
		dataStrNE(m, "doc_type", d.DocType)
		dataMap(m, "extracted_fields", d.ExtractedFields)
		// Plain JSON string (the old code JSON-marshaled the Go string, so the
		// JSON value is a quoted string — keep that shape).
		dataStrNE(m, "raw_extraction", d.RawExtraction)
		dataFloat64(m, "confidence", d.Confidence)
		dataStrNE(m, "user_directive", d.UserDirective)
		if d.PendingChoice != nil {
			m["pending_choice"] = map[string]any{
				"state":      d.PendingChoice.State,
				"outcome":    d.PendingChoice.Outcome,
				"created_at": d.PendingChoice.CreatedAt.Format(time.RFC3339),
				"expires_at": d.PendingChoice.ExpiresAt.Format(time.RFC3339),
			}
		}
		return m, nil
	},
	unmarshal: func(data map[string]any, d *entity.Document) error {
		if v, ok := data["doc_type"].(string); ok {
			d.DocType = v
		}
		unMap(data, "extracted_fields", &d.ExtractedFields)
		if v, ok := data["raw_extraction"].(string); ok {
			d.RawExtraction = v
		}
		unFloat64(data, "confidence", &d.Confidence)
		if v, ok := data["user_directive"].(string); ok {
			d.UserDirective = v
		}
		if v, ok := data["pending_choice"].(map[string]any); ok {
			pc := &entity.PendingChoice{}
			if s, ok2 := v["state"].(string); ok2 {
				pc.State = s
			}
			if s, ok2 := v["outcome"].(string); ok2 {
				pc.Outcome = s
			}
			if s, ok2 := v["created_at"].(string); ok2 {
				if t, err := time.Parse(time.RFC3339, s); err == nil {
					pc.CreatedAt = t
				}
			}
			if s, ok2 := v["expires_at"].(string); ok2 {
				if t, err := time.Parse(time.RFC3339, s); err == nil {
					pc.ExpiresAt = t
				}
			}
			d.PendingChoice = pc
		}
		return nil
	},
}

var accountFieldCols = map[string]string{
	"name":                "(payload #>> '{data,name}')",
	"account_type":        "(payload #>> '{data,account_type}')",
	"currency":            "(payload #>> '{data,currency}')",
	"institution":         "(payload #>> '{data,institution}')",
	"external_descriptor": "(payload #>> '{data,external_descriptor}')",
	"created_at":          "created_at",
	"updated_at":          "updated_at",
}

var accountOrderCols = map[string]string{
	"id":         "id",
	"created_at": "created_at",
	"updated_at": "updated_at",
	"name":       "(payload #>> '{data,name}')",
}

var accountCodec = &payloadCodec[entity.FinancialAccount]{
	table:      "financial_accounts",
	selectCols: []string{"id", "owner_id", "owner_household_id", "deleted_at", "created_at", "updated_at", "payload"},
	filters:    filterConfig{fieldCols: accountFieldCols, orderCols: accountOrderCols},
	columns: func(a entity.FinancialAccount) map[string]any {
		m := make(map[string]any)
		if a.OwnerHouseholdID != nil {
			m["owner_household_id"] = a.OwnerHouseholdID
		}
		return m
	},
	marshal: func(a entity.FinancialAccount) (map[string]any, []string) {
		m := make(map[string]any)
		dataStrNE(m, "name", a.Name)
		dataStrNE(m, "account_type", a.Type)
		dataStrNE(m, "currency", a.Currency)
		dataStr(m, "institution", a.Institution)
		dataStr(m, "external_descriptor", a.ExternalDescriptor)
		return m, nil
	},
	unmarshal: func(data map[string]any, a *entity.FinancialAccount) error {
		if v, ok := data["name"].(string); ok {
			a.Name = v
		}
		if v, ok := data["account_type"].(string); ok {
			a.Type = v
		}
		if v, ok := data["currency"].(string); ok {
			a.Currency = v
		}
		unStr(data, "institution", &a.Institution)
		unStr(data, "external_descriptor", &a.ExternalDescriptor)
		return nil
	},
}

var movementFieldCols = map[string]string{
	"kind":                   "(payload #>> '{data,kind}')",
	"amount":                 "((payload #>> '{data,amount}')::numeric)",
	"currency":               "(payload #>> '{data,currency}')",
	"occurred_on":            "((payload #>> '{data,occurred_on}')::date)",
	"recorded_at":            "((payload #>> '{data,recorded_at}')::timestamptz)",
	"description":            "(payload #>> '{data,description}')",
	"norm_description":       "(payload #>> '{data,norm_description}')",
	"origin":                 "(payload #>> '{data,origin}')",
	"source_account_id":      "source_account_id",
	"destination_account_id": "destination_account_id",
	"import_batch_id":        "import_batch_id",
	"external_reference":     "(payload #>> '{data,external_reference}')",
	"linked_document_id":     "linked_document_id",
	"created_at":             "created_at",
	"updated_at":             "updated_at",
}

var movementOrderCols = map[string]string{
	"id":          "id",
	"created_at":  "created_at",
	"updated_at":  "updated_at",
	"occurred_on": "((payload #>> '{data,occurred_on}')::date)",
	"kind":        "(payload #>> '{data,kind}')",
	"origin":      "(payload #>> '{data,origin}')",
}

var movementCodec = &payloadCodec[entity.MoneyMovement]{
	table:      "money_movements",
	selectCols: []string{"id", "owner_id", "owner_household_id", "source_account_id", "destination_account_id", "import_batch_id", "linked_document_id", "deleted_at", "created_at", "updated_at", "payload"},
	filters:    filterConfig{fieldCols: movementFieldCols, orderCols: movementOrderCols},
	columns: func(m entity.MoneyMovement) map[string]any {
		out := make(map[string]any)
		if m.OwnerHouseholdID != nil {
			out["owner_household_id"] = m.OwnerHouseholdID
		}
		if m.ID != "" {
			// Full fetched entity being updated: include the ID-based link
			// columns AS-IS so nil pointers map to SQL NULL (preserved exactly
			// from the old moneyMovementToMap).
			out["source_account_id"] = m.SourceAccountID
			out["destination_account_id"] = m.DestinationAccountID
			out["import_batch_id"] = m.ImportBatchID
			out["linked_document_id"] = m.LinkedDocumentID
		} else {
			// No-ID (create) path: non-nil only.
			if m.SourceAccountID != nil {
				out["source_account_id"] = m.SourceAccountID
			}
			if m.DestinationAccountID != nil {
				out["destination_account_id"] = m.DestinationAccountID
			}
			if m.ImportBatchID != nil {
				out["import_batch_id"] = m.ImportBatchID
			}
			if m.LinkedDocumentID != nil {
				out["linked_document_id"] = m.LinkedDocumentID
			}
		}
		return out
	},
	marshal: func(m entity.MoneyMovement) (map[string]any, []string) {
		mm := make(map[string]any)
		dataStrNE(mm, "kind", m.Kind)
		dataMoney(mm, "amount", m.Amount)
		dataStrNE(mm, "currency", m.Currency)
		dataDate(mm, "occurred_on", m.OccurredOn)
		dataTS(mm, "recorded_at", m.RecordedAt)
		if m.Description != "" {
			mm["description"] = m.Description
			mm["norm_description"] = normDescriptionOf(m.NormDescription, m.Description)
		}
		dataStrNE(mm, "origin", m.Origin)
		dataIntPtr(mm, "import_line", m.ImportLine)
		dataStr(mm, "external_reference", m.ExternalReference)
		dataStr(mm, "link_creator", m.LinkCreator)

		var clear []string
		if m.ID != "" {
			// Full entity: link_conflicting is written verbatim (false included),
			// and nil clearable pointer fields request removal of the stored key.
			mm["link_conflicting"] = m.LinkConflicting
			if m.ExternalReference == nil {
				clear = append(clear, "external_reference")
			}
			if m.LinkCreator == nil {
				clear = append(clear, "link_creator")
			}
			if m.ImportLine == nil {
				clear = append(clear, "import_line")
			}
		} else {
			// Partial (no ID): link_conflicting only when true; no clearing.
			dataBool(mm, "link_conflicting", m.LinkConflicting)
		}
		return mm, clear
	},
	unmarshal: func(data map[string]any, m *entity.MoneyMovement) error {
		if v, ok := data["kind"].(string); ok {
			m.Kind = v
		}
		unMoneyVal(data, "amount", &m.Amount)
		if v, ok := data["currency"].(string); ok {
			m.Currency = v
		}
		unDateVal(data, "occurred_on", &m.OccurredOn)
		unTSVal(data, "recorded_at", &m.RecordedAt)
		if v, ok := data["description"].(string); ok {
			m.Description = v
		}
		if v, ok := data["norm_description"].(string); ok {
			m.NormDescription = v
		}
		if v, ok := data["origin"].(string); ok {
			m.Origin = v
		}
		unInt(data, "import_line", &m.ImportLine)
		unStr(data, "external_reference", &m.ExternalReference)
		unStr(data, "link_creator", &m.LinkCreator)
		unBool(data, "link_conflicting", &m.LinkConflicting)
		return nil
	},
}

var importBatchFieldCols = map[string]string{
	"state":                "(payload #>> '{data,state}')",
	"account_id":           "account_id",
	"source_id":            "source_id",
	"filename":             "(payload #>> '{data,filename}')",
	"format":               "(payload #>> '{data,format}')",
	"line_count_valid":     "((payload #>> '{data,line_count_valid}')::int)",
	"line_count_duplicate": "((payload #>> '{data,line_count_duplicate}')::int)",
	"created_at":           "created_at",
	"updated_at":           "updated_at",
}

var importBatchOrderCols = map[string]string{
	"id":         "id",
	"created_at": "created_at",
	"updated_at": "updated_at",
	"state":      "(payload #>> '{data,state}')",
}

var importBatchCodec = &payloadCodec[entity.ImportBatch]{
	table:      "import_batches",
	selectCols: []string{"id", "owner_id", "owner_household_id", "account_id", "source_id", "deleted_at", "created_at", "updated_at", "payload"},
	filters:    filterConfig{fieldCols: importBatchFieldCols, orderCols: importBatchOrderCols},
	columns: func(b entity.ImportBatch) map[string]any {
		m := make(map[string]any)
		if b.OwnerHouseholdID != nil {
			m["owner_household_id"] = b.OwnerHouseholdID
		}
		if b.AccountID != "" {
			m["account_id"] = b.AccountID
		}
		if b.SourceID != "" {
			m["source_id"] = b.SourceID
		}
		return m
	},
	marshal: func(b entity.ImportBatch) (map[string]any, []string) {
		m := make(map[string]any)
		dataStrNE(m, "state", b.State)
		dataStrNE(m, "filename", b.Filename)
		dataStrNE(m, "format", b.Format)
		dataInt(m, "line_count_valid", b.LineCountValid)
		dataInt(m, "line_count_duplicate", b.LineCountDuplicate)
		dataInt(m, "line_count_possible_dup", b.LineCountPossibleDup)
		dataInt(m, "line_count_error", b.LineCountError)
		return m, nil
	},
	unmarshal: func(data map[string]any, b *entity.ImportBatch) error {
		if v, ok := data["state"].(string); ok {
			b.State = v
		}
		if v, ok := data["filename"].(string); ok {
			b.Filename = v
		}
		if v, ok := data["format"].(string); ok {
			b.Format = v
		}
		setCount := func(key string, p *int) {
			if v, ok := data[key]; ok {
				if n, ok2 := numInt64(v); ok2 {
					*p = int(n)
				}
			}
		}
		setCount("line_count_valid", &b.LineCountValid)
		setCount("line_count_duplicate", &b.LineCountDuplicate)
		setCount("line_count_possible_dup", &b.LineCountPossibleDup)
		setCount("line_count_error", &b.LineCountError)
		return nil
	},
}

var importLineFieldCols = map[string]string{
	"batch_id":           "import_batch_id",
	"line_ref":           "line_ref",
	"raw_line":           "(payload #>> '{data,raw_line}')",
	"occurred_on":        "((payload #>> '{data,occurred_on}')::date)",
	"amount":             "((payload #>> '{data,amount}')::numeric)",
	"direction":          "(payload #>> '{data,direction}')",
	"description":        "(payload #>> '{data,description}')",
	"norm_description":   "(payload #>> '{data,norm_description}')",
	"external_reference": "(payload #>> '{data,external_reference}')",
	"status":             "(payload #>> '{data,status}')",
	"created_at":         "created_at",
}

var importLineOrderCols = map[string]string{
	"id":         "id",
	"created_at": "created_at",
	"batch_id":   "import_batch_id",
	"line_ref":   "line_ref",
	"status":     "(payload #>> '{data,status}')",
}

var importLineCodec = &payloadCodec[entity.ImportLine]{
	table:      "import_lines",
	selectCols: []string{"id", "owner_id", "owner_household_id", "import_batch_id", "line_ref", "deleted_at", "created_at", "updated_at", "payload"},
	filters:    filterConfig{fieldCols: importLineFieldCols, orderCols: importLineOrderCols},
	columns: func(l entity.ImportLine) map[string]any {
		m := make(map[string]any)
		if l.OwnerHouseholdID != nil {
			m["owner_household_id"] = l.OwnerHouseholdID
		}
		if l.BatchID != "" {
			m["import_batch_id"] = l.BatchID
		}
		if l.LineRef != 0 {
			m["line_ref"] = l.LineRef
		}
		return m
	},
	marshal: func(l entity.ImportLine) (map[string]any, []string) {
		m := make(map[string]any)
		dataStrNE(m, "raw_line", l.RawLine)
		dataDatePtr(m, "occurred_on", l.OccurredOn)
		dataMoney(m, "amount", strDeref(l.Amount))
		if l.Direction != nil && *l.Direction != "" {
			m["direction"] = *l.Direction
		}
		if l.Description != nil {
			m["description"] = *l.Description
			if l.NormDescription != nil {
				m["norm_description"] = *l.NormDescription
			} else {
				m["norm_description"] = commons.NormalizeDescription(*l.Description)
			}
		}
		dataStr(m, "external_reference", l.ExternalReference)
		dataStrNE(m, "status", l.Status)
		dataStr(m, "error_reason", l.ErrorReason)
		return m, nil
	},
	unmarshal: func(data map[string]any, l *entity.ImportLine) error {
		if v, ok := data["raw_line"].(string); ok {
			l.RawLine = v
		}
		unDate(data, "occurred_on", &l.OccurredOn)
		unMoney(data, "amount", &l.Amount)
		unStr(data, "direction", &l.Direction)
		unStr(data, "description", &l.Description)
		unStr(data, "norm_description", &l.NormDescription)
		unStr(data, "external_reference", &l.ExternalReference)
		if v, ok := data["status"].(string); ok {
			l.Status = v
		}
		unStr(data, "error_reason", &l.ErrorReason)
		return nil
	},
}

var reviewFieldCols = map[string]string{
	"id":                    "id",
	"state":                 "(payload #>> '{data,state}')",
	"created_at":            "created_at",
	"source_id":             "source_id",
	"doc_type":              "(payload #>> '{data,doc_type}')",
	"owner_household_id":    "owner_household_id",
	"best_matched_asset_id": "(payload #>> '{data,best_matched_asset_id}')",
	"decided_at":            "((payload #>> '{data,decided_at}')::timestamptz)",
}

var reviewOrderCols = map[string]string{
	"id":         "id",
	"created_at": "created_at",
	"state":      "(payload #>> '{data,state}')",
}

var reviewCodec = &payloadCodec[entity.IngestReview]{
	table:      "ingest_reviews",
	selectCols: []string{"id", "owner_id", "owner_household_id", "source_id", "deleted_at", "created_at", "updated_at", "payload"},
	filters:    filterConfig{fieldCols: reviewFieldCols, orderCols: reviewOrderCols},
	columns: func(r entity.IngestReview) map[string]any {
		m := make(map[string]any)
		if r.OwnerHouseholdID != nil {
			m["owner_household_id"] = r.OwnerHouseholdID
		}
		if r.SourceID != "" {
			m["source_id"] = r.SourceID
		}
		return m
	},
	marshal: func(r entity.IngestReview) (map[string]any, []string) {
		m := make(map[string]any)
		dataStrNE(m, "doc_type", r.DocType)
		dataMap(m, "candidate_fields", r.CandidateFields)
		// Plain JSON string, matching the old JSON-marshaled-string shape.
		dataStrNE(m, "raw_extraction", r.RawExtraction)
		dataFloat64(m, "confidence", r.Confidence)
		dataStr(m, "best_matched_asset_id", r.BestMatchedAssetID)
		dataStrNE(m, "state", string(r.State))
		dataTSPtr(m, "decided_at", r.DecidedAt)
		dataStr(m, "decided_by", r.DecidedBy)
		dataMap(m, "provenance", r.Provenance)
		return m, nil
	},
	unmarshal: func(data map[string]any, r *entity.IngestReview) error {
		if v, ok := data["doc_type"].(string); ok {
			r.DocType = v
		}
		unMap(data, "candidate_fields", &r.CandidateFields)
		if v, ok := data["raw_extraction"].(string); ok {
			r.RawExtraction = v
		}
		unFloat64(data, "confidence", &r.Confidence)
		unStr(data, "best_matched_asset_id", &r.BestMatchedAssetID)
		if v, ok := data["state"].(string); ok {
			r.State = entity.IngestReviewState(v)
		}
		unTS(data, "decided_at", &r.DecidedAt)
		unStr(data, "decided_by", &r.DecidedBy)
		unMap(data, "provenance", &r.Provenance)
		return nil
	},
}

var householdFieldCols = map[string]string{
	"display_name": "(payload #>> '{data,display_name}')",
	"owner_id":     "owner_id",
	"created_at":   "created_at",
}

var householdOrderCols = map[string]string{
	"id":         "id",
	"created_at": "created_at",
}

var householdCodec = &payloadCodec[entity.Household]{
	table:      "households",
	selectCols: []string{"id", "owner_id", "deleted_at", "created_at", "updated_at", "payload"},
	filters:    filterConfig{fieldCols: householdFieldCols, orderCols: householdOrderCols},
	columns: func(h entity.Household) map[string]any {
		// No non-payload columns beyond owner_id (the engine adds it).
		return make(map[string]any)
	},
	marshal: func(h entity.Household) (map[string]any, []string) {
		m := make(map[string]any)
		dataStrNE(m, "display_name", h.DisplayName)
		return m, nil
	},
	unmarshal: func(data map[string]any, h *entity.Household) error {
		if v, ok := data["display_name"].(string); ok {
			h.DisplayName = v
		}
		return nil
	},
}

// strDeref returns the dereferenced string or "" for nil.
func strDeref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
