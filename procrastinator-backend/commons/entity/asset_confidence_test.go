package entity

import "testing"

func TestAssetConfidence(t *testing.T) {
	t.Parallel()

	t.Run("nil means not scored", func(t *testing.T) {
		t.Parallel()
		a := Asset{}
		if a.Confidence != nil {
			t.Fatalf("Asset.Confidence = %v, want nil for zero-value Asset", *a.Confidence)
		}
	})

	t.Run("stores score", func(t *testing.T) {
		t.Parallel()
		want := 0.72
		a := Asset{Confidence: ptrToFloat(want)}
		if a.Confidence == nil || *a.Confidence != want {
			t.Fatalf("Asset.Confidence = %v, want %v", ptrFloatDisplay(a.Confidence), want)
		}
	})
}

func TestDocumentConfidence(t *testing.T) {
	t.Parallel()

	t.Run("nil means not scored", func(t *testing.T) {
		t.Parallel()
		d := Document{}
		if d.Confidence != nil {
			t.Fatalf("Document.Confidence = %v, want nil for zero-value Document", *d.Confidence)
		}
	})

	t.Run("stores score", func(t *testing.T) {
		t.Parallel()
		want := 0.95
		d := Document{Confidence: ptrToFloat(want)}
		if d.Confidence == nil || *d.Confidence != want {
			t.Fatalf("Document.Confidence = %v, want %v", ptrFloatDisplay(d.Confidence), want)
		}
	})

	t.Run("DocumentWithSource inherits Confidence", func(t *testing.T) {
		t.Parallel()
		want := 0.5
		dws := DocumentWithSource{
			Document: Document{Confidence: ptrToFloat(want)},
		}
		if dws.Confidence == nil || *dws.Confidence != want {
			t.Fatalf("DocumentWithSource.Confidence = %v, want %v", ptrFloatDisplay(dws.Confidence), want)
		}
	})
}
