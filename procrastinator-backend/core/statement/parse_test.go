package statement

import (
	"encoding/csv"
	"strings"
	"testing"
	"time"
)

// reparseCSV re-parses a single-line raw string as one CSV record for
// round-trip verification.
func reparseCSV(t *testing.T, raw string) []string {
	t.Helper()
	r := csv.NewReader(strings.NewReader(raw))
	r.FieldsPerRecord = -1
	recs, err := r.ReadAll()
	if err != nil {
		t.Fatalf("re-parsing raw line %q failed: %v", raw, err)
	}
	if len(recs) != 1 {
		t.Fatalf("re-parsing raw line %q returned %d records, want 1", raw, len(recs))
	}
	return recs[0]
}

type parseCSVCase struct {
	name    string
	data    string
	wantLen int
	wantErr bool
	// wantRaw, when set, is the exact expected raw string for the row at
	// wantRawIndex.
	wantRaw      string
	wantRawIndex int
	// wantFields, when set, is the expected re-parsed fields for the row at
	// wantFieldsIndex.
	wantFields      []string
	wantFieldsIndex int
}

func TestParseCSV(t *testing.T) {
	t.Parallel()

	cases := []parseCSVCase{
		{
			name:    "multiple rows round-trip",
			data:    "2024-01-05,1500.50,Reliance Digital,REF-1\n2024-01-06,-200.00,\"Office, Stationery Dept\"",
			wantLen: 2,
		},
		{
			name:    "empty nil input",
			data:    "",
			wantLen: 0,
		},
		{
			name:    "whitespace-only input",
			data:    "   \n  ",
			wantLen: 0,
		},
		{
			name:    "header row skipped",
			data:    "date,amount,description,reference\n2024-01-05,1500.50,Reliance Digital,REF-1",
			wantLen: 1,
		},
		{
			name:    "headerless all rows returned",
			data:    "2024-01-05,1500.50,Reliance Digital\n2024-01-06,-200.00,Coffee Shop",
			wantLen: 2,
		},
		{
			name:    "bare quote is an error",
			data:    "2024-01-05,1500.50,un\"balanced\n",
			wantErr: true,
		},
		{
			name:            "description with comma stays quoted",
			data:            "2024-01-06,-200.00,\"Office, Stationery Dept\"",
			wantLen:         1,
			wantRaw:         `2024-01-06,-200.00,"Office, Stationery Dept"`,
			wantRawIndex:    0,
			wantFields:      []string{"2024-01-06", "-200.00", "Office, Stationery Dept"},
			wantFieldsIndex: 0,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var data []byte
			if tc.name != "empty nil input" {
				data = []byte(tc.data)
			}
			got, err := ParseCSV(data)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ParseCSV returned nil error, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseCSV returned error %v, want nil", err)
			}
			if len(got) != tc.wantLen {
				t.Fatalf("len(lines) = %d, want %d (lines: %q)", len(got), tc.wantLen, got)
			}
			if tc.wantRaw != "" {
				if got[tc.wantRawIndex] != tc.wantRaw {
					t.Fatalf("raw[%d] = %q, want %q", tc.wantRawIndex, got[tc.wantRawIndex], tc.wantRaw)
				}
			}
			if tc.wantFields != nil {
				fields := reparseCSV(t, got[tc.wantFieldsIndex])
				if len(fields) != len(tc.wantFields) {
					t.Fatalf("re-parsed fields = %v, want %v", fields, tc.wantFields)
				}
				for i := range fields {
					if fields[i] != tc.wantFields[i] {
						t.Fatalf("re-parsed field %d = %q, want %q", i, fields[i], tc.wantFields[i])
					}
				}
			}
		})
	}
}

type extractCase struct {
	name           string
	lineRef        int
	raw            string
	wantStatus     string
	wantDate       *string // ISO date string when OccurredOn set
	wantAmount     *string
	wantDirection  string
	wantDesc       *string
	wantNorm       *string
	wantExtRef     *string
	wantErrMention string // substring that must appear in ErrorReason; "" = ErrorReason must be nil
}

func (c extractCase) check(t *testing.T, p ParsedLine) {
	t.Helper()
	if p.LineRef != c.lineRef {
		t.Fatalf("LineRef = %d, want %d", p.LineRef, c.lineRef)
	}
	if p.RawLine != c.raw {
		t.Fatalf("RawLine = %q, want %q", p.RawLine, c.raw)
	}
	if p.Status != c.wantStatus {
		t.Fatalf("Status = %q, want %q", p.Status, c.wantStatus)
	}
	if c.wantErrMention != "" {
		if p.ErrorReason == nil {
			t.Fatalf("ErrorReason = nil, want non-nil mentioning %q", c.wantErrMention)
		}
		if !strings.Contains(*p.ErrorReason, c.wantErrMention) {
			t.Fatalf("ErrorReason = %q, want it to mention %q", *p.ErrorReason, c.wantErrMention)
		}
	} else if p.ErrorReason != nil {
		t.Fatalf("ErrorReason = %q, want nil", *p.ErrorReason)
	}
	if c.wantDate != nil {
		if p.OccurredOn == nil {
			t.Fatalf("OccurredOn = nil, want %q", *c.wantDate)
		}
		got := p.OccurredOn.Format("2006-01-02")
		if got != *c.wantDate {
			t.Fatalf("OccurredOn = %q, want %q", got, *c.wantDate)
		}
	} else if p.OccurredOn != nil {
		t.Fatalf("OccurredOn = %v, want nil", *p.OccurredOn)
	}
	if c.wantAmount != nil {
		if p.Amount == nil {
			t.Fatalf("Amount = nil, want %q", *c.wantAmount)
		}
		if *p.Amount != *c.wantAmount {
			t.Fatalf("Amount = %q, want %q", *p.Amount, *c.wantAmount)
		}
	} else if p.Amount != nil {
		t.Fatalf("Amount = %q, want nil", *p.Amount)
	}
	if p.Direction != c.wantDirection {
		t.Fatalf("Direction = %q, want %q", p.Direction, c.wantDirection)
	}
	if c.wantDesc != nil {
		if p.Description == nil {
			t.Fatalf("Description = nil, want %q", *c.wantDesc)
		}
		if *p.Description != *c.wantDesc {
			t.Fatalf("Description = %q, want %q", *p.Description, *c.wantDesc)
		}
	} else if p.Description != nil {
		t.Fatalf("Description = %q, want nil", *p.Description)
	}
	if c.wantNorm != nil {
		if p.NormDescription == nil {
			t.Fatalf("NormDescription = nil, want %q", *c.wantNorm)
		}
		if *p.NormDescription != *c.wantNorm {
			t.Fatalf("NormDescription = %q, want %q", *p.NormDescription, *c.wantNorm)
		}
	} else if p.NormDescription != nil {
		t.Fatalf("NormDescription = %q, want nil", *p.NormDescription)
	}
	if c.wantExtRef != nil {
		if p.ExternalReference == nil {
			t.Fatalf("ExternalReference = nil, want %q", *c.wantExtRef)
		}
		if *p.ExternalReference != *c.wantExtRef {
			t.Fatalf("ExternalReference = %q, want %q", *p.ExternalReference, *c.wantExtRef)
		}
	} else if p.ExternalReference != nil {
		t.Fatalf("ExternalReference = %q, want nil", *p.ExternalReference)
	}
}

func sp(s string) *string { return &s }

func TestExtractFields(t *testing.T) {
	t.Parallel()

	cases := []extractCase{
		{
			name:          "valid 3 fields",
			lineRef:       1,
			raw:           `2024-01-05,1500.50,  Reliance   Digital `,
			wantStatus:    "valid",
			wantDate:      sp("2024-01-05"),
			wantAmount:    sp("1500.50"),
			wantDirection: "in",
			wantDesc:      sp("Reliance   Digital"),
			wantNorm:      sp("reliance digital"),
		},
		{
			name:          "valid 4 fields with external reference",
			lineRef:       2,
			raw:           "2024-01-06,-200.00,Coffee Shop,REF-99",
			wantStatus:    "valid",
			wantDate:      sp("2024-01-06"),
			wantAmount:    sp("200.00"),
			wantDirection: "out",
			wantDesc:      sp("Coffee Shop"),
			wantNorm:      sp("coffee shop"),
			wantExtRef:    sp("REF-99"),
		},
		{
			name:          "negative amount is out and absolute",
			lineRef:       3,
			raw:           "2024-01-07,-0.01,Charge",
			wantStatus:    "valid",
			wantDate:      sp("2024-01-07"),
			wantAmount:    sp("0.01"),
			wantDirection: "out",
			wantDesc:      sp("Charge"),
			wantNorm:      sp("charge"),
		},
		{
			name:          "positive sign accepted",
			lineRef:       4,
			raw:           "2024-01-08,+50.00,Deposit",
			wantStatus:    "valid",
			wantDate:      sp("2024-01-08"),
			wantAmount:    sp("50.00"),
			wantDirection: "in",
			wantDesc:      sp("Deposit"),
			wantNorm:      sp("deposit"),
		},
		{
			name:           "missing date",
			lineRef:        5,
			raw:            ",1500.50,Reliance Digital",
			wantStatus:     "error",
			wantAmount:     sp("1500.50"),
			wantDirection:  "in",
			wantDesc:       sp("Reliance Digital"),
			wantNorm:       sp("reliance digital"),
			wantErrMention: "date",
		},
		{
			name:           "unparseable date",
			lineRef:        6,
			raw:            "not-a-date,1500.50,Reliance Digital",
			wantStatus:     "error",
			wantAmount:     sp("1500.50"),
			wantDirection:  "in",
			wantDesc:       sp("Reliance Digital"),
			wantNorm:       sp("reliance digital"),
			wantErrMention: "date",
		},
		{
			name:           "missing amount",
			lineRef:        7,
			raw:            "2024-01-05,,Reliance Digital",
			wantStatus:     "error",
			wantDate:       sp("2024-01-05"),
			wantDesc:       sp("Reliance Digital"),
			wantNorm:       sp("reliance digital"),
			wantErrMention: "amount",
		},
		{
			name:           "invalid amount abc",
			lineRef:        8,
			raw:            "2024-01-05,abc,Reliance Digital",
			wantStatus:     "error",
			wantDate:       sp("2024-01-05"),
			wantDesc:       sp("Reliance Digital"),
			wantNorm:       sp("reliance digital"),
			wantErrMention: "amount",
		},
		{
			name:           "invalid amount zero",
			lineRef:        9,
			raw:            "2024-01-05,0,Reliance Digital",
			wantStatus:     "error",
			wantDate:       sp("2024-01-05"),
			wantDesc:       sp("Reliance Digital"),
			wantNorm:       sp("reliance digital"),
			wantErrMention: "amount",
		},
		{
			name:           "invalid amount zero decimals",
			lineRef:        10,
			raw:            "2024-01-05,0.00,Reliance Digital",
			wantStatus:     "error",
			wantDate:       sp("2024-01-05"),
			wantDesc:       sp("Reliance Digital"),
			wantNorm:       sp("reliance digital"),
			wantErrMention: "amount",
		},
		{
			name:           "invalid amount two dots",
			lineRef:        11,
			raw:            "2024-01-05,12.5.3,Reliance Digital",
			wantStatus:     "error",
			wantDate:       sp("2024-01-05"),
			wantDesc:       sp("Reliance Digital"),
			wantNorm:       sp("reliance digital"),
			wantErrMention: "amount",
		},
		{
			name:           "missing description",
			lineRef:        12,
			raw:            "2024-01-05,1500.50,",
			wantStatus:     "error",
			wantDate:       sp("2024-01-05"),
			wantAmount:     sp("1500.50"),
			wantDirection:  "in",
			wantErrMention: "description",
		},
		{
			name:           "missing description whitespace only",
			lineRef:        121,
			raw:            "2024-01-05,1500.50,   ",
			wantStatus:     "error",
			wantDate:       sp("2024-01-05"),
			wantAmount:     sp("1500.50"),
			wantDirection:  "in",
			wantErrMention: "description",
		},
		{
			name:           "two fields",
			lineRef:        13,
			raw:            "2024-01-05,1500.50",
			wantStatus:     "error",
			wantErrMention: "fields",
		},
		{
			name:           "five fields",
			lineRef:        14,
			raw:            "2024-01-05,1500.50,Reliance Digital,REF-1,EXTRA",
			wantStatus:     "error",
			wantErrMention: "fields",
		},
		{
			name:           "malformed raw bare quote",
			lineRef:        15,
			raw:            `2024-01-05,1500.50,un"balanced`,
			wantStatus:     "error",
			wantErrMention: "malformed",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := ExtractFields(tc.lineRef, tc.raw)
			tc.check(t, p)
		})
	}
}

// TestExtractFieldsNeverPanics guards the contract that ExtractFields never
// panics even on hostile input.
func TestExtractFieldsNeverPanics(t *testing.T) {
	t.Parallel()

	inputs := []string{
		"",
		"\x00\x01binary",
		`"`,
		`"unclosed`,
		`2024-01-05,1500.50,de"sc,ription",extra",more",`,
		strings.Repeat("x", 10000),
	}
	for _, in := range inputs {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("ExtractFields panicked on %q: %v", in, r)
				}
			}()
			_ = ExtractFields(1, in)
		}()
	}
}

// TestParsedLineFieldsLocked guards the exact ParsedLine field order and the
// direction constants against accidental drift.
func TestParsedLineFieldsLocked(t *testing.T) {
	t.Parallel()

	if DirectionIn != "in" {
		t.Fatalf("DirectionIn = %q, want \"in\"", DirectionIn)
	}
	if DirectionOut != "out" {
		t.Fatalf("DirectionOut = %q, want \"out\"", DirectionOut)
	}
	if FormatCSV != "csv" {
		t.Fatalf("FormatCSV = %q, want \"csv\"", FormatCSV)
	}
	if FormatPDF != "pdf" {
		t.Fatalf("FormatPDF = %q, want \"pdf\"", FormatPDF)
	}

	p := ParsedLine{
		LineRef:           7,
		RawLine:           "raw",
		OccurredOn:        spTime(time.Date(2024, 1, 5, 0, 0, 0, 0, time.UTC)),
		Amount:            sp("1.00"),
		Direction:         DirectionOut,
		Description:       sp("d"),
		NormDescription:   sp("d"),
		ExternalReference: sp("ref"),
		Status:            "valid",
	}
	if p.LineRef != 7 || p.RawLine != "raw" || *p.OccurredOn != time.Date(2024, 1, 5, 0, 0, 0, 0, time.UTC) ||
		*p.Amount != "1.00" || p.Direction != DirectionOut || *p.Description != "d" ||
		*p.NormDescription != "d" || *p.ExternalReference != "ref" || p.Status != "valid" ||
		p.ErrorReason != nil {
		t.Fatalf("ParsedLine fields do not round-trip: %+v", p)
	}
}

// TestPDFTextExtractorInterface guards the consumer-side interface shape.
func TestPDFTextExtractorInterface(t *testing.T) {
	t.Parallel()

	var _ PDFTextExtractor = (*stubPDFTextExtractor)(nil)
}

type stubPDFTextExtractor struct{}

func (s *stubPDFTextExtractor) ExtractText(data []byte) (string, error) {
	return string(data), nil
}

func spTime(v time.Time) *time.Time { return &v }
