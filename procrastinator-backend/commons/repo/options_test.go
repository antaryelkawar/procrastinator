package repo

import (
	"reflect"
	"testing"
)

func TestOwner(t *testing.T) {
	t.Parallel()

	got := ApplyOptions(Owner("t1"))

	if got.OwnerID != "t1" {
		t.Errorf("OwnerID = %q, want %q", got.OwnerID, "t1")
	}
}

func TestWhere(t *testing.T) {
	t.Parallel()

	got := ApplyOptions(Where("brand", "=", "Samsung"))

	want := []Filter{{Field: "brand", Op: "=", Value: "Samsung"}}
	if !reflect.DeepEqual(got.Filters, want) {
		t.Errorf("Filters = %+v, want %+v", got.Filters, want)
	}
}

func TestWhereMultiple(t *testing.T) {
	t.Parallel()

	got := ApplyOptions(
		Where("brand", "=", "Samsung"),
		Where("status", "!=", "archived"),
	)

	want := []Filter{
		{Field: "brand", Op: "=", Value: "Samsung"},
		{Field: "status", Op: "!=", Value: "archived"},
	}
	if len(got.Filters) != 2 {
		t.Fatalf("len(Filters) = %d, want 2", len(got.Filters))
	}
	if !reflect.DeepEqual(got.Filters, want) {
		t.Errorf("Filters = %+v, want %+v (order must be preserved)", got.Filters, want)
	}
}

func TestLimit(t *testing.T) {
	t.Parallel()

	got := ApplyOptions(Limit(10))

	if got.Limit != 10 {
		t.Errorf("Limit = %d, want 10", got.Limit)
	}
}

func TestOffset(t *testing.T) {
	t.Parallel()

	got := ApplyOptions(Offset(5))

	if got.Offset != 5 {
		t.Errorf("Offset = %d, want 5", got.Offset)
	}
}

func TestOrderBy(t *testing.T) {
	t.Parallel()

	got := ApplyOptions(OrderBy("created_at"))

	if got.OrderBy != "created_at" {
		t.Errorf("OrderBy = %q, want %q", got.OrderBy, "created_at")
	}
}

func TestApplyOptions_Multiple(t *testing.T) {
	t.Parallel()

	got := ApplyOptions(
		Owner("t1"),
		Where("brand", "=", "Samsung"),
		Limit(10),
		Offset(5),
		OrderBy("created_at"),
	)

	want := &Options{
		OwnerID: "t1",
		Filters: []Filter{{Field: "brand", Op: "=", Value: "Samsung"}},
		Limit:   10,
		Offset:  5,
		OrderBy: "created_at",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ApplyOptions(...) = %+v, want %+v", got, want)
	}
}

func TestApplyOptions_NoOptions(t *testing.T) {
	t.Parallel()

	got := ApplyOptions()

	want := &Options{}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ApplyOptions() = %+v, want zero-value %+v", got, want)
	}
}
