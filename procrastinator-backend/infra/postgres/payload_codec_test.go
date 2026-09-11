package postgres

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"procrastinator-backend/commons"
	"procrastinator-backend/commons/entity"
)

// TestPayloadCodecMoneyRoundTrip verifies money fields survive a
// marshal→unmarshal cycle as exact decimals: the marshaled JSON must carry the
// value as a JSON number (never a string, never a float64-rounded value) and
// the unmarshaled entity must hold the exact original decimal string.
func TestPayloadCodecMoneyRoundTrip(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		amount string
	}{
		{"typical", "39999.99"},
		{"large", "123456789.10"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			data, _ := movementCodec.marshal(entity.MoneyMovement{
				Amount:   tc.amount,
				Currency: "INR",
			})

			// The data map must carry the amount as a json.Number, so the
			// marshaled bytes contain the exact decimal as a JSON number.
			n, ok := data["amount"].(json.Number)
			if !ok {
				t.Fatalf("amount = %T (%v), want json.Number", data["amount"], data["amount"])
			}
			raw, err := json.Marshal(map[string]any{"amount": n})
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			want := `{"amount":` + tc.amount + `}`
			if string(raw) != want {
				t.Errorf("marshaled JSON = %s, want %s", raw, want)
			}

			var got entity.MoneyMovement
			if err := movementCodec.unmarshal(data, &got); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if got.Amount != tc.amount {
				t.Errorf("Amount = %q, want %q", got.Amount, tc.amount)
			}
			if got.Currency != "INR" {
				t.Errorf("Currency = %q, want INR", got.Currency)
			}
		})
	}
}

// TestPayloadCodecFullEntityRoundTrip builds a fully-populated entity of every
// kind, marshals it to the data map, unmarshals into a fresh entity, and
// compares all data fields. The movement subcase also verifies the full-entity
// (ID set) clearing semantics: nil clearable fields produce clear keys, and a
// merge-then-unmarshal of the merged data object clears them.
func TestPayloadCodecFullEntityRoundTrip(t *testing.T) {
	t.Parallel()

	created := time.Date(2024, 1, 12, 10, 30, 0, 0, time.UTC)
	mergeTS := time.Date(2024, 5, 1, 8, 0, 0, 0, time.UTC)
	uploaded := time.Date(2024, 2, 3, 4, 5, 6, 0, time.UTC)
	occurred := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	lineOccurred := time.Date(2024, 3, 4, 0, 0, 0, 0, time.UTC)
	decided := time.Date(2024, 6, 7, 9, 8, 7, 0, time.UTC)

	t.Run("asset", func(t *testing.T) {
		t.Parallel()
		// Dates are stored as "2006-01-02": the time component is truncated,
		// so the round-tripped entity must use midnight dates. The norm
		// fields are derived by the codec from the raw fields (preserved
		// old toMap behavior), so the expectation carries the derived norms.
		purchaseDay := time.Date(2024, 1, 12, 0, 0, 0, 0, time.UTC)
		warrantyDay := time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC)
		orig := entity.Asset{
			ID:                 "asset-1",
			OwnerID:            "owner-1",
			OwnerHouseholdID:   strPtr("hh-1"),
			Name:               strPtr("  Smart  TV "),
			NormName:           strPtr(commons.NormalizeName("  Smart  TV ")),
			Brand:              strPtr("Samsung"),
			NormBrand:          strPtr(commons.NormalizeName("Samsung")),
			Model:              strPtr("QN85"),
			NormModel:          strPtr(commons.NormalizeName("QN85")),
			SerialNumber:       strPtr("  AB-12 CD"),
			NormSerial:         strPtr(commons.NormalizeSerial("  AB-12 CD")),
			PurchaseDate:       &purchaseDay,
			WarrantyEnd:        &warrantyDay,
			Price:              strPtr("12345.67"),
			Currency:           strPtr("INR"),
			Metadata:           map[string]any{"color": "black"},
			Confidence:         f64ptr(0.93),
			AssetCategory:      strPtr("electronics"),
			CategoryConfidence: f64ptr(0.8),
			CategoryUserSet:    true,
			MergedInto:         strPtr("asset-9"),
			MergedAt:           &mergeTS,
		}
		assertCodecRoundTrip(t, assetCodec, assetData, orig)
	})

	t.Run("source", func(t *testing.T) {
		t.Parallel()
		orig := entity.Source{
			ID:               "src-1",
			OwnerID:          "owner-1",
			OwnerHouseholdID: strPtr("hh-1"),
			Filename:         "invoice.pdf",
			ContentType:      "application/pdf",
			Size:             123456,
			Path:             "/files/invoice.pdf",
			SHA256:           "abc123",
			UploadedAt:       uploaded,
		}
		assertCodecRoundTrip(t, sourceCodec, sourceData, orig)
	})

	t.Run("document", func(t *testing.T) {
		t.Parallel()
		orig := entity.Document{
			ID:               "doc-1",
			OwnerID:          "owner-1",
			OwnerHouseholdID: strPtr("hh-1"),
			AssetID:          "asset-1",
			SourceID:         "src-1",
			DocType:          "invoice",
			ExtractedFields:  map[string]any{"brand": "Samsung"},
			RawExtraction:    "raw extraction text",
			Confidence:       f64ptr(0.7),
			UserDirective:    "check the serial number",
		}
		assertCodecRoundTrip(t, documentCodec, documentData, orig)
	})

	t.Run("account", func(t *testing.T) {
		t.Parallel()
		orig := entity.FinancialAccount{
			ID:                 "acct-1",
			OwnerID:            "owner-1",
			OwnerHouseholdID:   strPtr("hh-1"),
			Name:               "Savings",
			Type:               "bank",
			Currency:           "INR",
			Institution:        strPtr("HDFC"),
			ExternalDescriptor: strPtr("desc-1"),
		}
		assertCodecRoundTrip(t, accountCodec, accountData, orig)
	})

	t.Run("movement full entity", func(t *testing.T) {
		t.Parallel()
		orig := entity.MoneyMovement{
			ID:                   "mov-1",
			OwnerID:              "owner-1",
			OwnerHouseholdID:     strPtr("hh-1"),
			Kind:                 "expense",
			Amount:               "39999.99",
			Currency:             "INR",
			OccurredOn:           occurred,
			RecordedAt:           created,
			Description:          "Groceries at  Big Bazaar",
			// Derived by the codec from Description (preserved old toMap rule).
			NormDescription:      commons.NormalizeDescription("Groceries at  Big Bazaar"),
			Origin:               "manual",
			SourceAccountID:      strPtr("acct-1"),
			DestinationAccountID: nil, // column, nil → SQL NULL on full update
			ImportBatchID:        nil, // column, nil → SQL NULL on full update
			ImportLine:           nil, // clearable data key, nil → cleared
			ExternalReference:    nil, // clearable data key, nil → cleared
			LinkedDocumentID:     strPtr("doc-1"),
			LinkCreator:          nil, // clearable data key, nil → cleared
			LinkConflicting:      false,
		}
		assertCodecRoundTrip(t, movementCodec, movementData, orig)

		// Clearing semantics: full entity (ID set) with nil clearable data
		// fields must request removal of exactly the nil clearable data keys.
		// The account/import-batch/linked-document ids are real columns (the
		// engine's columns map handles them), never data keys.
		data, clear := movementCodec.marshal(orig)
		if _, ok := data["destination_account_id"]; ok {
			t.Errorf("full entity: destination_account_id present in data (%v), want absent", data["destination_account_id"])
		}
		wantClear := []string{"external_reference", "import_line", "link_creator"}
		if len(clear) != len(wantClear) {
			t.Fatalf("clear = %v, want %v", clear, wantClear)
		}
		clearSet := make(map[string]bool, len(clear))
		for _, k := range clear {
			clearSet[k] = true
		}
		for _, k := range wantClear {
			if !clearSet[k] {
				t.Errorf("clear missing key %q: %v", k, clear)
			}
		}
		if clearSet["linked_document_id"] {
			t.Errorf("clear must not contain linked_document_id: %v", clear)
		}
		if v, ok := data["link_conflicting"]; !ok || v != false {
			t.Errorf("full entity: link_conflicting = %v (%T), want false present", v, v)
		}

		// Merge simulation: the stored data object still holds the previously
		// set values for the clearable keys; applying the clear list must drop
		// them, and unmarshaling the merged object must yield nil fields.
		// (Account/import-batch/linked-document ids are real columns, not data
		// keys, so they are not part of the payload merge.)
		stored := map[string]any{
			"kind":               "expense",
			"amount":             json.Number("39999.99"),
			"currency":           "INR",
			"description":        "Groceries at  Big Bazaar",
			"norm_description":   commons.NormalizeDescription("Groceries at  Big Bazaar"),
			"origin":             "manual",
			"import_line":        json.Number("7"),
			"external_reference": "REF-1",
			"link_creator":       "manual",
			"link_conflicting":   true,
		}
		for k, v := range data {
			stored[k] = v
		}
		for _, k := range clear {
			delete(stored, k)
		}
		var got entity.MoneyMovement
		if err := movementCodec.unmarshal(stored, &got); err != nil {
			t.Fatalf("unmarshal merged: %v", err)
		}
		if got.ImportLine != nil {
			t.Errorf("after merge, ImportLine = %v, want nil", *got.ImportLine)
		}
		if got.ExternalReference != nil {
			t.Errorf("after merge, ExternalReference = %v, want nil", *got.ExternalReference)
		}
		if got.LinkCreator != nil {
			t.Errorf("after merge, LinkCreator = %v, want nil", *got.LinkCreator)
		}
		if got.LinkConflicting {
			t.Errorf("after merge, LinkConflicting = true, want false (full entity writes false verbatim)")
		}
	})

	t.Run("movement partial no-ID", func(t *testing.T) {
		t.Parallel()
		orig := entity.MoneyMovement{
			OwnerID:           "owner-1",
			OwnerHouseholdID:  strPtr("hh-1"),
			Kind:              "income",
			Amount:            "100.00",
			Currency:          "INR",
			OccurredOn:        occurred,
			RecordedAt:        created,
			Description:       "Salary",
			// Derived by the codec from Description (preserved old toMap rule).
			NormDescription:   commons.NormalizeDescription("Salary"),
			Origin:            "import",
			ImportBatchID:     strPtr("batch-1"),
			ImportLine:        intPtr(3),
			ExternalReference: strPtr("REF-2"),
			LinkConflicting:   true,
		}
		assertCodecRoundTrip(t, movementCodec, movementData, orig)

		data, clear := movementCodec.marshal(orig)
		if len(clear) != 0 {
			t.Errorf("partial entity must not clear: clear = %v", clear)
		}
		if _, ok := data["destination_account_id"]; ok {
			t.Errorf("partial entity: destination_account_id present, want absent")
		}
		if _, ok := data["linked_document_id"]; ok {
			t.Errorf("partial entity: linked_document_id present, want absent")
		}
		if _, ok := data["link_creator"]; ok {
			t.Errorf("partial entity: link_creator present, want absent")
		}
		if v, ok := data["link_conflicting"]; !ok || v != true {
			t.Errorf("partial entity: link_conflicting = %v, want true", v)
		}
	})

	t.Run("import_batch", func(t *testing.T) {
		t.Parallel()
		orig := entity.ImportBatch{
			ID:                   "batch-1",
			OwnerID:              "owner-1",
			OwnerHouseholdID:     strPtr("hh-1"),
			State:                "preview",
			AccountID:            "acct-1",
			SourceID:             "src-1",
			Filename:             "statement.csv",
			Format:               "csv",
			LineCountValid:       10,
			LineCountDuplicate:   2,
			LineCountPossibleDup: 1,
			LineCountError:       0, // zero → omitted, not part of data
		}
		assertCodecRoundTrip(t, importBatchCodec, importBatchData, orig)
	})

	t.Run("import_line", func(t *testing.T) {
		t.Parallel()
		orig := entity.ImportLine{
			ID:                "line-1",
			OwnerID:           "owner-1",
			OwnerHouseholdID:  strPtr("hh-1"),
			BatchID:           "batch-1",
			LineRef:           7,
			RawLine:           "01/15/2024,DEBIT,39999.99,Groceries at  Big Bazaar",
			OccurredOn:        &lineOccurred,
			Amount:            strPtr("39999.99"),
			Direction:         strPtr("debit"),
			Description:       strPtr("Groceries at  Big Bazaar"),
			// Derived by the codec from Description (preserved old toMap rule).
			NormDescription:   strPtr(commons.NormalizeDescription("Groceries at  Big Bazaar")),
			ExternalReference: strPtr("REF-3"),
			Status:            "valid",
			ErrorReason:       nil, // zero → omitted
		}
		assertCodecRoundTrip(t, importLineCodec, importLineData, orig)
	})

	t.Run("review", func(t *testing.T) {
		t.Parallel()
		orig := entity.IngestReview{
			ID:                 "rev-1",
			OwnerID:            "owner-1",
			OwnerHouseholdID:   strPtr("hh-1"),
			SourceID:           "src-1",
			DocType:            "invoice",
			CandidateFields:    map[string]any{"brand": "Samsung", "model": "WF80A"},
			RawExtraction:      "raw review extraction",
			Confidence:         f64ptr(0.42),
			BestMatchedAssetID: strPtr("asset-1"),
			State:              entity.ReviewStateApproved,
			DecidedAt:          &decided,
			DecidedBy:          strPtr("decider"),
			Provenance:         map[string]any{"workers": []any{"w1", "w2"}},
		}
		assertCodecRoundTrip(t, reviewCodec, reviewData, orig)
	})

	t.Run("household", func(t *testing.T) {
		t.Parallel()
		orig := entity.Household{
			ID:          "hh-1",
			OwnerID:     "owner-1",
			DisplayName: "The  Smiths ",
		}
		assertCodecRoundTrip(t, householdCodec, householdData, orig)
	})
}

// assertCodecRoundTrip marshals orig to the data map, unmarshals into a fresh
// entity, and compares every data field. project keeps only the data fields
// (column fields like ID/OwnerID/OwnerHouseholdID are not part of the data
// object, so they must be excluded from the comparison).
func assertCodecRoundTrip[T any](t *testing.T, c *payloadCodec[T], project func(T) T, orig T) {
	t.Helper()
	data, _ := c.marshal(orig)
	var got T
	if err := c.unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !reflect.DeepEqual(project(got), project(orig)) {
		t.Errorf("round-trip mismatch:\n got  %+v\n want %+v", project(got), project(orig))
	}
}

// Per-kind data-field projectors: everything except the physical columns
// (id, owner_id, owner_household_id and the kind's FK/link columns).

func assetData(a entity.Asset) entity.Asset {
	return entity.Asset{
		Brand:              a.Brand,
		Model:              a.Model,
		SerialNumber:       a.SerialNumber,
		NormSerial:         a.NormSerial,
		NormBrand:          a.NormBrand,
		NormModel:          a.NormModel,
		NormName:           a.NormName,
		Name:               a.Name,
		PurchaseDate:       a.PurchaseDate,
		WarrantyEnd:        a.WarrantyEnd,
		Price:              a.Price,
		Currency:           a.Currency,
		Metadata:           a.Metadata,
		Confidence:         a.Confidence,
		AssetCategory:      a.AssetCategory,
		CategoryConfidence: a.CategoryConfidence,
		CategoryUserSet:    a.CategoryUserSet,
		MergedInto:         a.MergedInto,
		MergedAt:           a.MergedAt,
	}
}

func sourceData(s entity.Source) entity.Source {
	return entity.Source{
		Filename:    s.Filename,
		ContentType: s.ContentType,
		Size:        s.Size,
		Path:        s.Path,
		SHA256:      s.SHA256,
		UploadedAt:  s.UploadedAt,
	}
}

func documentData(d entity.Document) entity.Document {
	return entity.Document{
		DocType:         d.DocType,
		ExtractedFields: d.ExtractedFields,
		RawExtraction:   d.RawExtraction,
		Confidence:      d.Confidence,
		UserDirective:   d.UserDirective,
	}
}

func accountData(a entity.FinancialAccount) entity.FinancialAccount {
	return entity.FinancialAccount{
		Name:               a.Name,
		Type:               a.Type,
		Currency:           a.Currency,
		Institution:        a.Institution,
		ExternalDescriptor: a.ExternalDescriptor,
	}
}

func movementData(m entity.MoneyMovement) entity.MoneyMovement {
	return entity.MoneyMovement{
		Kind:              m.Kind,
		Amount:            m.Amount,
		Currency:          m.Currency,
		OccurredOn:        m.OccurredOn,
		RecordedAt:        m.RecordedAt,
		Description:       m.Description,
		NormDescription:   m.NormDescription,
		Origin:            m.Origin,
		ImportLine:        m.ImportLine,
		ExternalReference: m.ExternalReference,
		LinkCreator:       m.LinkCreator,
		LinkConflicting:   m.LinkConflicting,
	}
}

func importBatchData(b entity.ImportBatch) entity.ImportBatch {
	return entity.ImportBatch{
		State:                b.State,
		Filename:             b.Filename,
		Format:               b.Format,
		LineCountValid:       b.LineCountValid,
		LineCountDuplicate:   b.LineCountDuplicate,
		LineCountPossibleDup: b.LineCountPossibleDup,
		LineCountError:       b.LineCountError,
	}
}

func importLineData(l entity.ImportLine) entity.ImportLine {
	return entity.ImportLine{
		RawLine:           l.RawLine,
		OccurredOn:        l.OccurredOn,
		Amount:            l.Amount,
		Direction:         l.Direction,
		Description:       l.Description,
		NormDescription:   l.NormDescription,
		ExternalReference: l.ExternalReference,
		Status:            l.Status,
		ErrorReason:       l.ErrorReason,
	}
}

func reviewData(r entity.IngestReview) entity.IngestReview {
	return entity.IngestReview{
		DocType:            r.DocType,
		CandidateFields:    r.CandidateFields,
		RawExtraction:      r.RawExtraction,
		Confidence:         r.Confidence,
		BestMatchedAssetID: r.BestMatchedAssetID,
		State:              r.State,
		DecidedAt:          r.DecidedAt,
		DecidedBy:          r.DecidedBy,
		Provenance:         r.Provenance,
	}
}

func householdData(h entity.Household) entity.Household {
	return entity.Household{DisplayName: h.DisplayName}
}

// TestPayloadCodecInclusionRules verifies zero-value entities marshal to an
// empty data map, and partial entities include only the set fields (plus
// derived norm fields where the old toMap derived them).
func TestPayloadCodecInclusionRules(t *testing.T) {
	t.Parallel()

	t.Run("zero value empty", func(t *testing.T) {
		t.Parallel()
		marshalers := map[string]func() (map[string]any, []string){
			"asset":        func() (map[string]any, []string) { return assetCodec.marshal(entity.Asset{}) },
			"source":       func() (map[string]any, []string) { return sourceCodec.marshal(entity.Source{}) },
			"document":     func() (map[string]any, []string) { return documentCodec.marshal(entity.Document{}) },
			"account":      func() (map[string]any, []string) { return accountCodec.marshal(entity.FinancialAccount{}) },
			"movement":     func() (map[string]any, []string) { return movementCodec.marshal(entity.MoneyMovement{}) },
			"import_batch": func() (map[string]any, []string) { return importBatchCodec.marshal(entity.ImportBatch{}) },
			"import_line":  func() (map[string]any, []string) { return importLineCodec.marshal(entity.ImportLine{}) },
			"review":       func() (map[string]any, []string) { return reviewCodec.marshal(entity.IngestReview{}) },
			"household":    func() (map[string]any, []string) { return householdCodec.marshal(entity.Household{}) },
		}
		for name, m := range marshalers {
			data, clear := m()
			if len(data) != 0 {
				t.Errorf("%s: zero-value data = %v, want empty", name, data)
			}
			if len(clear) != 0 {
				t.Errorf("%s: zero-value clear = %v, want empty", name, clear)
			}
		}
	})

	t.Run("asset brand only derives norm brand", func(t *testing.T) {
		t.Parallel()
		data, _ := assetCodec.marshal(entity.Asset{Brand: strPtr("  Samsung ")})
		want := map[string]any{"brand": "  Samsung ", "norm_brand": commons.NormalizeName("  Samsung ")}
		if !reflect.DeepEqual(data, want) {
			t.Errorf("data = %v, want %v", data, want)
		}
	})

	t.Run("asset norm set with raw nil", func(t *testing.T) {
		t.Parallel()
		data, _ := assetCodec.marshal(entity.Asset{NormSerial: strPtr("AB12CD")})
		want := map[string]any{"norm_serial": "AB12CD"}
		if !reflect.DeepEqual(data, want) {
			t.Errorf("data = %v, want %v", data, want)
		}
	})

	t.Run("movement partial only set fields", func(t *testing.T) {
		t.Parallel()
		data, _ := movementCodec.marshal(entity.MoneyMovement{
			Kind:              "expense",
			ExternalReference: strPtr("REF-9"),
		})
		want := map[string]any{"kind": "expense", "external_reference": "REF-9"}
		if !reflect.DeepEqual(data, want) {
			t.Errorf("data = %v, want %v", data, want)
		}
	})

	t.Run("source size zero omitted", func(t *testing.T) {
		t.Parallel()
		data, _ := sourceCodec.marshal(entity.Source{Filename: "f.pdf", Size: 0})
		want := map[string]any{"filename": "f.pdf"}
		if !reflect.DeepEqual(data, want) {
			t.Errorf("data = %v, want %v", data, want)
		}
	})
}

// TestPayloadCodecDateTimestampRoundTrip verifies "2006-01-02" dates and
// RFC3339 timestamps survive a marshal→unmarshal cycle exactly.
func TestPayloadCodecDateTimestampRoundTrip(t *testing.T) {
	t.Parallel()

	t.Run("date", func(t *testing.T) {
		t.Parallel()
		d := time.Date(2024, 1, 12, 0, 0, 0, 0, time.UTC)
		a := entity.Asset{PurchaseDate: &d}
		data, _ := assetCodec.marshal(a)
		if data["purchase_date"] != "2024-01-12" {
			t.Errorf("purchase_date = %v, want %q", data["purchase_date"], "2024-01-12")
		}
		var got entity.Asset
		if err := assetCodec.unmarshal(data, &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if got.PurchaseDate == nil || !got.PurchaseDate.Equal(d) {
			t.Errorf("PurchaseDate = %v, want %v", got.PurchaseDate, d)
		}

		// Movement occurred_on (value time.Time) round-trips too.
		m := entity.MoneyMovement{OccurredOn: d}
		mdata, _ := movementCodec.marshal(m)
		var gotM entity.MoneyMovement
		if err := movementCodec.unmarshal(mdata, &gotM); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if !gotM.OccurredOn.Equal(d) {
			t.Errorf("OccurredOn = %v, want %v", gotM.OccurredOn, d)
		}
	})

	t.Run("timestamp", func(t *testing.T) {
		t.Parallel()
		ts := time.Date(2024, 7, 8, 9, 10, 11, 0, time.UTC)
		s := entity.Source{UploadedAt: ts}
		data, _ := sourceCodec.marshal(s)
		if data["uploaded_at"] != "2024-07-08T09:10:11Z" {
			t.Errorf("uploaded_at = %v, want RFC3339", data["uploaded_at"])
		}
		var got entity.Source
		if err := sourceCodec.unmarshal(data, &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if !got.UploadedAt.Equal(ts) {
			t.Errorf("UploadedAt = %v, want %v", got.UploadedAt, ts)
		}

		// Asset merged_at (*time.Time) round-trips too.
		a := entity.Asset{MergedAt: &ts}
		adata, _ := assetCodec.marshal(a)
		var gotA entity.Asset
		if err := assetCodec.unmarshal(adata, &gotA); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if gotA.MergedAt == nil || !gotA.MergedAt.Equal(ts) {
			t.Errorf("MergedAt = %v, want %v", gotA.MergedAt, ts)
		}
	})

	t.Run("zero timestamp absent", func(t *testing.T) {
		t.Parallel()
		data, _ := sourceCodec.marshal(entity.Source{Filename: "f"})
		if _, ok := data["uploaded_at"]; ok {
			t.Errorf("zero UploadedAt must be absent from data: %v", data)
		}
	})
}

// TestPayloadCodecUnknownKeysIgnored verifies unmarshaling a data object with
// extra keys does not error and leaves the entity fields for those keys
// untouched.
func TestPayloadCodecUnknownKeysIgnored(t *testing.T) {
	t.Parallel()

	data := map[string]any{
		"name":           "Microwave",
		"future_field_x": "ignored",
		"future_field_y": 42,
		"future_field_z": map[string]any{"deep": true},
	}
	var got entity.Asset
	if err := assetCodec.unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal with unknown keys: %v", err)
	}
	if got.Name == nil || *got.Name != "Microwave" {
		t.Errorf("Name = %v, want Microwave", got.Name)
	}

	// A pre-populated entity keeps its fields untouched by unknown keys.
	pre := entity.Asset{Brand: strPtr("keep")}
	if err := assetCodec.unmarshal(data, &pre); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if pre.Brand == nil || *pre.Brand != "keep" {
		t.Errorf("Brand = %v, want keep", pre.Brand)
	}
}

// TestPayloadCodecSelectCols pins the per-kind SELECT column list (the engine
// contract for the generic engine in the next chunk).
func TestPayloadCodecSelectCols(t *testing.T) {
	t.Parallel()

	want := map[string]string{
		"assets":             "id, owner_id, owner_household_id, deleted_at, created_at, updated_at, payload",
		"sources":            "id, owner_id, owner_household_id, deleted_at, created_at, updated_at, payload",
		"documents":          "id, owner_id, owner_household_id, asset_id, source_id, deleted_at, created_at, updated_at, payload",
		"financial_accounts": "id, owner_id, owner_household_id, deleted_at, created_at, updated_at, payload",
		"money_movements":    "id, owner_id, owner_household_id, source_account_id, destination_account_id, import_batch_id, linked_document_id, deleted_at, created_at, updated_at, payload",
		"import_batches":     "id, owner_id, owner_household_id, account_id, source_id, deleted_at, created_at, updated_at, payload",
		"import_lines":       "id, owner_id, owner_household_id, import_batch_id, line_ref, deleted_at, created_at, updated_at, payload",
		"ingest_reviews":     "id, owner_id, owner_household_id, source_id, deleted_at, created_at, updated_at, payload",
		"households":         "id, owner_id, deleted_at, created_at, updated_at, payload",
	}
	codecs := []struct {
		table      string
		selectCols []string
	}{
		{assetCodec.table, assetCodec.selectCols},
		{sourceCodec.table, sourceCodec.selectCols},
		{documentCodec.table, documentCodec.selectCols},
		{accountCodec.table, accountCodec.selectCols},
		{movementCodec.table, movementCodec.selectCols},
		{importBatchCodec.table, importBatchCodec.selectCols},
		{importLineCodec.table, importLineCodec.selectCols},
		{reviewCodec.table, reviewCodec.selectCols},
		{householdCodec.table, householdCodec.selectCols},
	}
	seen := make(map[string]bool, len(codecs))
	for _, c := range codecs {
		if seen[c.table] {
			t.Errorf("duplicate table %q in codec registry", c.table)
		}
		seen[c.table] = true
		wantCols, ok := want[c.table]
		if !ok {
			t.Errorf("codec table %q has no expected column list", c.table)
			continue
		}
		if gotCols := strings.Join(c.selectCols, ", "); gotCols != wantCols {
			t.Errorf("table %q selectCols = %q, want %q", c.table, gotCols, wantCols)
		}
	}
}

// intPtr is a test helper for *int fields (ImportLine).
func intPtr(i int) *int { return &i }
