package extraction

import (
	"bytes"
	"errors"
	"testing"
	"time"
)

// TestParseExtraction_FullValidPayload ensures every field is populated
// correctly from a complete, well-formed JSON payload.
func TestParseExtraction_FullValidPayload(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		raw  []byte
		want Extraction
	}{
		{
			name: "invoice full payload",
			raw:  []byte(`{"document_type":"invoice","brand":"Samsung","model":"WW90T534DAW","serial_number":"SN-123-ABC","purchase_date":"2024-03-15","price":"1299.99","currency":"EUR","warranty_start":"2024-03-15","warranty_end":"2026-03-15"}`),
			want: Extraction{
				Classification: "invoice",
				Brand:          "Samsung",
				Model:          "WW90T534DAW",
				SerialNumber:   "SN-123-ABC",
				PurchaseDate:   time.Date(2024, time.March, 15, 0, 0, 0, 0, time.UTC),
				Price:          "1299.99",
				Currency:       "EUR",
				WarrantyStart:  time.Date(2024, time.March, 15, 0, 0, 0, 0, time.UTC),
				WarrantyEnd:    time.Date(2026, time.March, 15, 0, 0, 0, 0, time.UTC),
				RawPayload:     `{"document_type":"invoice","brand":"Samsung","model":"WW90T534DAW","serial_number":"SN-123-ABC","purchase_date":"2024-03-15","price":"1299.99","currency":"EUR","warranty_start":"2024-03-15","warranty_end":"2026-03-15"}`,
			},
		},
		{
			name: "warranty full payload",
			raw:  []byte(`{"document_type":"warranty","brand":"Acme","model":"W-200","serial_number":"ACME-77","purchase_date":"2023-11-01","price":"89","currency":"USD","warranty_start":"2023-11-01","warranty_end":"2025-11-01"}`),
			want: Extraction{
				Classification: "warranty",
				Brand:          "Acme",
				Model:          "W-200",
				SerialNumber:   "ACME-77",
				PurchaseDate:   time.Date(2023, time.November, 1, 0, 0, 0, 0, time.UTC),
				Price:          "89",
				Currency:       "USD",
				WarrantyStart:  time.Date(2023, time.November, 1, 0, 0, 0, 0, time.UTC),
				WarrantyEnd:    time.Date(2025, time.November, 1, 0, 0, 0, 0, time.UTC),
				RawPayload:     `{"document_type":"warranty","brand":"Acme","model":"W-200","serial_number":"ACME-77","purchase_date":"2023-11-01","price":"89","currency":"USD","warranty_start":"2023-11-01","warranty_end":"2025-11-01"}`,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseExtraction(tc.raw)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Classification != tc.want.Classification {
				t.Errorf("Classification: got %q, want %q", got.Classification, tc.want.Classification)
			}
			if got.Brand != tc.want.Brand {
				t.Errorf("Brand: got %q, want %q", got.Brand, tc.want.Brand)
			}
			if got.Model != tc.want.Model {
				t.Errorf("Model: got %q, want %q", got.Model, tc.want.Model)
			}
			if got.SerialNumber != tc.want.SerialNumber {
				t.Errorf("SerialNumber: got %q, want %q", got.SerialNumber, tc.want.SerialNumber)
			}
			if !got.PurchaseDate.Equal(tc.want.PurchaseDate) {
				t.Errorf("PurchaseDate: got %v, want %v", got.PurchaseDate, tc.want.PurchaseDate)
			}
			if got.Price != tc.want.Price {
				t.Errorf("Price: got %q, want %q", got.Price, tc.want.Price)
			}
			if got.Currency != tc.want.Currency {
				t.Errorf("Currency: got %q, want %q", got.Currency, tc.want.Currency)
			}
			if !got.WarrantyStart.Equal(tc.want.WarrantyStart) {
				t.Errorf("WarrantyStart: got %v, want %v", got.WarrantyStart, tc.want.WarrantyStart)
			}
			if !got.WarrantyEnd.Equal(tc.want.WarrantyEnd) {
				t.Errorf("WarrantyEnd: got %v, want %v", got.WarrantyEnd, tc.want.WarrantyEnd)
			}
			if got.RawPayload != tc.want.RawPayload {
				t.Errorf("RawPayload: got %q, want %q", got.RawPayload, tc.want.RawPayload)
			}
		})
	}
}

// TestParseExtraction_ThoughtStripping ensures <thought>...</thought> blocks
// are stripped before parsing while RawPayload preserves the original input.
func TestParseExtraction_ThoughtStripping(t *testing.T) {
	t.Parallel()

	const pureJSON = `{"document_type":"warranty","brand":"Acme","model":"W-200","serial_number":"ACME-77","purchase_date":"2023-11-01","price":"89","currency":"USD","warranty_start":"2023-11-01","warranty_end":"2025-11-01"}`

	ref, err := ParseExtraction([]byte(pureJSON))
	if err != nil {
		t.Fatalf("failed to parse pureJSON reference: %v", err)
	}

	cases := []struct {
		name string
		raw  []byte
	}{
		{
			name: "pure JSON baseline",
			raw:  []byte(pureJSON),
		},
		{
			name: "single thought before JSON",
			raw:  []byte(`<thought>The document looks like a warranty card, so I will classify it as warranty.</thought>` + pureJSON),
		},
		{
			name: "thought with surrounding whitespace",
			raw:  []byte(`  <thought>  hmm  </thought>  ` + pureJSON + `  `),
		},
		{
			name: "multiple consecutive thought blocks",
			raw:  []byte(`<thought>first</thought><thought>second</thought><thought>third</thought>` + pureJSON),
		},
		{
			name: "multiline thought containing JSON-like content",
			raw: []byte(`<thought>
{"document_type":"invoice","brand":"TrapBrand"}
nested content with "quotes" and newlines
</thought>` + pureJSON),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseExtraction(tc.raw)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Classification != ref.Classification {
				t.Errorf("Classification: got %q, want %q", got.Classification, ref.Classification)
			}
			if got.Brand != ref.Brand {
				t.Errorf("Brand: got %q, want %q", got.Brand, ref.Brand)
			}
			if got.Model != ref.Model {
				t.Errorf("Model: got %q, want %q", got.Model, ref.Model)
			}
			if got.SerialNumber != ref.SerialNumber {
				t.Errorf("SerialNumber: got %q, want %q", got.SerialNumber, ref.SerialNumber)
			}
			if !got.PurchaseDate.Equal(ref.PurchaseDate) {
				t.Errorf("PurchaseDate: got %v, want %v", got.PurchaseDate, ref.PurchaseDate)
			}
			if got.Price != ref.Price {
				t.Errorf("Price: got %q, want %q", got.Price, ref.Price)
			}
			if got.Currency != ref.Currency {
				t.Errorf("Currency: got %q, want %q", got.Currency, ref.Currency)
			}
			if !got.WarrantyStart.Equal(ref.WarrantyStart) {
				t.Errorf("WarrantyStart: got %v, want %v", got.WarrantyStart, ref.WarrantyStart)
			}
			if !got.WarrantyEnd.Equal(ref.WarrantyEnd) {
				t.Errorf("WarrantyEnd: got %v, want %v", got.WarrantyEnd, ref.WarrantyEnd)
			}
			if got.RawPayload != string(tc.raw) {
				t.Errorf("RawPayload: got %q, want %q", got.RawPayload, string(tc.raw))
			}
		})
	}
}

// TestParseExtraction_Classification checks that the classification field
// accepts the three valid values and degrades everything else to "other".
func TestParseExtraction_Classification(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		raw  []byte
		want string
	}{
		{name: "invoice kept", raw: []byte(`{"document_type":"invoice"}`), want: "invoice"},
		{name: "warranty kept", raw: []byte(`{"document_type":"warranty"}`), want: "warranty"},
		{name: "other kept", raw: []byte(`{"document_type":"other"}`), want: "other"},
		{name: "unknown degrades to other", raw: []byte(`{"document_type":"receipt"}`), want: "other"},
		{name: "missing degrades to other", raw: []byte(`{}`), want: "other"},
		{name: "null degrades to other", raw: []byte(`{"document_type":null}`), want: "other"},
		{name: "empty string degrades to other", raw: []byte(`{"document_type":""}`), want: "other"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseExtraction(tc.raw)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Classification != tc.want {
				t.Errorf("Classification: got %q, want %q", got.Classification, tc.want)
			}
		})
	}
}

// TestParseExtraction_Dates validates the three date fields accept ISO 8601
// date-only and RFC 3339 formats and yield zero time on garbage input.
func TestParseExtraction_Dates(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name         string
		raw          []byte
		wantPurchase time.Time
		wantStart    time.Time
		wantEnd      time.Time
	}{
		{
			name:         "all three valid date-only",
			raw:          []byte(`{"purchase_date":"2024-03-15","warranty_start":"2024-03-15","warranty_end":"2026-03-15"}`),
			wantPurchase: time.Date(2024, time.March, 15, 0, 0, 0, 0, time.UTC),
			wantStart:    time.Date(2024, time.March, 15, 0, 0, 0, 0, time.UTC),
			wantEnd:      time.Date(2026, time.March, 15, 0, 0, 0, 0, time.UTC),
		},
		{
			name:         "garbage purchase date absent, others kept",
			raw:          []byte(`{"purchase_date":"sometime last year","warranty_start":"2024-03-15","warranty_end":"2026-03-15"}`),
			wantPurchase: time.Time{},
			wantStart:    time.Date(2024, time.March, 15, 0, 0, 0, 0, time.UTC),
			wantEnd:      time.Date(2026, time.March, 15, 0, 0, 0, 0, time.UTC),
		},
		{
			name:         "wrong layout absent",
			raw:          []byte(`{"purchase_date":"15-03-2024"}`),
			wantPurchase: time.Time{},
			wantStart:    time.Time{},
			wantEnd:      time.Time{},
		},
		{
			name:         "out-of-range absent",
			raw:          []byte(`{"purchase_date":"2024-13-45"}`),
			wantPurchase: time.Time{},
			wantStart:    time.Time{},
			wantEnd:      time.Time{},
		},
		{
			name:         "RFC3339 timestamp parses",
			raw:          []byte(`{"purchase_date":"2024-03-15T10:30:00Z"}`),
			wantPurchase: time.Date(2024, time.March, 15, 10, 30, 0, 0, time.UTC),
			wantStart:    time.Time{},
			wantEnd:      time.Time{},
		},
		{
			name:         "null dates absent",
			raw:          []byte(`{"purchase_date":null,"warranty_start":null,"warranty_end":null}`),
			wantPurchase: time.Time{},
			wantStart:    time.Time{},
			wantEnd:      time.Time{},
		},
		{
			name:         "empty string absent",
			raw:          []byte(`{"purchase_date":""}`),
			wantPurchase: time.Time{},
			wantStart:    time.Time{},
			wantEnd:      time.Time{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseExtraction(tc.raw)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !got.PurchaseDate.Equal(tc.wantPurchase) {
				t.Errorf("PurchaseDate: got %v, want %v", got.PurchaseDate, tc.wantPurchase)
			}
			if !got.WarrantyStart.Equal(tc.wantStart) {
				t.Errorf("WarrantyStart: got %v, want %v", got.WarrantyStart, tc.wantStart)
			}
			if !got.WarrantyEnd.Equal(tc.wantEnd) {
				t.Errorf("WarrantyEnd: got %v, want %v", got.WarrantyEnd, tc.wantEnd)
			}
		})
	}
}

// TestParseExtraction_Price ensures prices are validated as non-negative
// decimal strings and that JSON numbers are normalized to strings.
func TestParseExtraction_Price(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		raw  []byte
		want string
	}{
		{name: "string decimal kept exactly", raw: []byte(`{"price":"1299.99"}`), want: "1299.99"},
		{name: "integer string", raw: []byte(`{"price":"1299"}`), want: "1299"},
		{name: "small decimal", raw: []byte(`{"price":"0.99"}`), want: "0.99"},
		{name: "JSON number kept exactly", raw: []byte(`{"price":1299.99}`), want: "1299.99"},
		{name: "non-numeric absent", raw: []byte(`{"price":"abc"}`), want: ""},
		{name: "double dot absent", raw: []byte(`{"price":"12.34.56"}`), want: ""},
		{name: "thousands separator absent", raw: []byte(`{"price":"1,299.99"}`), want: ""},
		{name: "negative absent", raw: []byte(`{"price":"-5"}`), want: ""},
		{name: "empty string absent", raw: []byte(`{"price":""}`), want: ""},
		{name: "trailing space absent", raw: []byte(`{"price":"1299.99 "}`), want: ""},
		{name: "null absent", raw: []byte(`{"price":null}`), want: ""},
		{name: "missing absent", raw: []byte(`{}`), want: ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseExtraction(tc.raw)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Price != tc.want {
				t.Errorf("Price: got %q, want %q", got.Price, tc.want)
			}
		})
	}
}

// TestParseExtraction_Currency validates that only 3-letter ASCII currency
// codes are accepted and that they are normalized to uppercase.
func TestParseExtraction_Currency(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		raw  []byte
		want string
	}{
		{name: "uppercase kept", raw: []byte(`{"currency":"USD"}`), want: "USD"},
		{name: "lowercase uppercased", raw: []byte(`{"currency":"usd"}`), want: "USD"},
		{name: "mixed case uppercased", raw: []byte(`{"currency":"EuR"}`), want: "EUR"},
		{name: "two letters absent", raw: []byte(`{"currency":"US"}`), want: ""},
		{name: "four letters absent", raw: []byte(`{"currency":"USDX"}`), want: ""},
		{name: "digit absent", raw: []byte(`{"currency":"U1D"}`), want: ""},
		{name: "null absent", raw: []byte(`{"currency":null}`), want: ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseExtraction(tc.raw)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Currency != tc.want {
				t.Errorf("Currency: got %q, want %q", got.Currency, tc.want)
			}
		})
	}
}

// TestParseExtraction_NullFieldsAbsent confirms zero-value defaults
// when fields are missing or explicitly null.
func TestParseExtraction_NullFieldsAbsent(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		raw  []byte
		want Extraction
	}{
		{
			name: "only document_type",
			raw:  []byte(`{"document_type":"invoice"}`),
			want: Extraction{
				Classification: "invoice",
				Brand:          "",
				Model:          "",
				SerialNumber:   "",
				PurchaseDate:   time.Time{},
				Price:          "",
				Currency:       "",
				WarrantyStart:  time.Time{},
				WarrantyEnd:    time.Time{},
				RawPayload:     `{"document_type":"invoice"}`,
			},
		},
		{
			name: "empty object",
			raw:  []byte(`{}`),
			want: Extraction{
				Classification: "other",
				Brand:          "",
				Model:          "",
				SerialNumber:   "",
				PurchaseDate:   time.Time{},
				Price:          "",
				Currency:       "",
				WarrantyStart:  time.Time{},
				WarrantyEnd:    time.Time{},
				RawPayload:     `{}`,
			},
		},
		{
			name: "all fields null",
			raw:  []byte(`{"document_type":null,"brand":null,"model":null,"serial_number":null,"purchase_date":null,"price":null,"currency":null,"warranty_start":null,"warranty_end":null}`),
			want: Extraction{
				Classification: "other",
				Brand:          "",
				Model:          "",
				SerialNumber:   "",
				PurchaseDate:   time.Time{},
				Price:          "",
				Currency:       "",
				WarrantyStart:  time.Time{},
				WarrantyEnd:    time.Time{},
				RawPayload:     `{"document_type":null,"brand":null,"model":null,"serial_number":null,"purchase_date":null,"price":null,"currency":null,"warranty_start":null,"warranty_end":null}`,
			},
		},
		{
			name: "unknown extra fields ignored",
			raw:  []byte(`{"document_type":"invoice","extra_field":"ignored"}`),
			want: Extraction{
				Classification: "invoice",
				Brand:          "",
				Model:          "",
				SerialNumber:   "",
				PurchaseDate:   time.Time{},
				Price:          "",
				Currency:       "",
				WarrantyStart:  time.Time{},
				WarrantyEnd:    time.Time{},
				RawPayload:     `{"document_type":"invoice","extra_field":"ignored"}`,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseExtraction(tc.raw)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Classification != tc.want.Classification {
				t.Errorf("Classification: got %q, want %q", got.Classification, tc.want.Classification)
			}
			if got.Brand != tc.want.Brand {
				t.Errorf("Brand: got %q, want %q", got.Brand, tc.want.Brand)
			}
			if got.Model != tc.want.Model {
				t.Errorf("Model: got %q, want %q", got.Model, tc.want.Model)
			}
			if got.SerialNumber != tc.want.SerialNumber {
				t.Errorf("SerialNumber: got %q, want %q", got.SerialNumber, tc.want.SerialNumber)
			}
			if !got.PurchaseDate.Equal(tc.want.PurchaseDate) {
				t.Errorf("PurchaseDate: got %v, want %v", got.PurchaseDate, tc.want.PurchaseDate)
			}
			if got.Price != tc.want.Price {
				t.Errorf("Price: got %q, want %q", got.Price, tc.want.Price)
			}
			if got.Currency != tc.want.Currency {
				t.Errorf("Currency: got %q, want %q", got.Currency, tc.want.Currency)
			}
			if !got.WarrantyStart.Equal(tc.want.WarrantyStart) {
				t.Errorf("WarrantyStart: got %v, want %v", got.WarrantyStart, tc.want.WarrantyStart)
			}
			if !got.WarrantyEnd.Equal(tc.want.WarrantyEnd) {
				t.Errorf("WarrantyEnd: got %v, want %v", got.WarrantyEnd, tc.want.WarrantyEnd)
			}
			if got.RawPayload != tc.want.RawPayload {
				t.Errorf("RawPayload: got %q, want %q", got.RawPayload, tc.want.RawPayload)
			}
		})
	}
}

// TestParseExtraction_Malformed validates that unparseable input
// returns ErrUnparseable and a zero-value Extraction.
func TestParseExtraction_Malformed(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		raw  []byte
	}{
		{name: "plain text", raw: []byte(`this is not json`)},
		{name: "empty input", raw: []byte(``)},
		{name: "whitespace only", raw: []byte("   \n\t  ")},
		{name: "thought block only", raw: []byte(`<thought>I cannot find any JSON in this document</thought>`)},
		{name: "null literal", raw: []byte(`null`)},
		{name: "array", raw: []byte(`[1,2,3]`)},
		{name: "truncated object", raw: []byte(`{"document_type":"invoice"`)},
		{name: "trailing data", raw: []byte(`{"document_type":"invoice"} trailing`)},
		{name: "string literal", raw: []byte(`"invoice"`)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseExtraction(tc.raw)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !errors.Is(err, ErrUnparseable) {
				t.Errorf("expected errors.Is(err, ErrUnparseable), got %v", err)
			}
			var zero Extraction
			if got != zero {
				t.Errorf("expected zero Extraction, got %+v", got)
			}
		})
	}
}

// TestStripThoughts exercises the unexported stripThoughts helper
// directly to ensure it removes only lowercase <thought>...</thought> blocks
// non-greedily while preserving surrounding bytes verbatim.
func TestStripThoughts(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   []byte
		want []byte
	}{
		{
			name: "no thoughts unchanged",
			in:   []byte(`{"a":1}`),
			want: []byte(`{"a":1}`),
		},
		{
			name: "single block removed",
			in:   []byte(`<thought>think</thought>{"a":1}`),
			want: []byte(`{"a":1}`),
		},
		{
			name: "multiple blocks removed",
			in:   []byte(`<thought>a</thought><thought>b</thought>{"a":1}`),
			want: []byte(`{"a":1}`),
		},
		{
			name: "surrounding whitespace preserved outside blocks",
			in:   []byte(`  <thought>  x  </thought>  {"a":1}  `),
			want: []byte(`    {"a":1}  `),
		},
		{
			name: "multiline nested content removed",
			in:   []byte("<thought>\nline one {\"a\":1}\nline two\n</thought>{\"a\":1}"),
			want: []byte(`{"a":1}`),
		},
		{
			name: "unclosed tag left as-is",
			in:   []byte(`<thought>unclosed{"a":1}`),
			want: []byte(`<thought>unclosed{"a":1}`),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := stripThoughts(tc.in)
			if !bytes.Equal(got, tc.want) {
				t.Errorf("stripThoughts(%q) = %q, want %q", string(tc.in), string(got), string(tc.want))
			}
		})
	}
}
