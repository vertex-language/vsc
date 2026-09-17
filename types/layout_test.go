package types

import "testing"

// TestOffsetof: fields are laid out in the order they were written,
// each at the next offset its alignment admits. Nothing is reordered,
// so padding is visible to a program that looks for it.
func TestOffsetof(t *testing.T) {
	s := &Struct{Name: "S", Fields: []*Field{
		{Name: "a", Type: Typ[Int8]},
		{Name: "b", Type: Typ[Int64]},
		{Name: "c", Type: Typ[Int8]},
		{Name: "d", Type: Typ[Int32]},
	}}
	for _, c := range []struct {
		field string
		want  int64
	}{
		{"a", 0},
		{"b", 8}, // seven bytes of padding
		{"c", 16},
		{"d", 20}, // three more
	} {
		got, ok := Offsetof(s, c.field, DefaultTarget64)
		if !ok {
			t.Errorf("no field %q", c.field)
			continue
		}
		if got != c.want {
			t.Errorf("%s is at %d, want %d", c.field, got, c.want)
		}
	}
	if _, ok := Offsetof(s, "nope", DefaultTarget64); ok {
		t.Error("a field that is not there has an offset")
	}
	if _, ok := Offsetof(Typ[Int], "a", DefaultTarget64); ok {
		t.Error("an Int has a field")
	}
}

// TestOffsetofTuple: a tuple element is reachable by its position as
// well as by its label, which is how `t.0` is written.
func TestOffsetofTuple(t *testing.T) {
	tup := &Tuple{Elements: []*TupleElement{
		{Type: Typ[Int32]},
		{Name: "second", Type: Typ[Int64]},
	}}
	if got, ok := Offsetof(tup, "0", DefaultTarget64); !ok || got != 0 {
		t.Errorf("element 0 is at %d (%v)", got, ok)
	}
	if got, ok := Offsetof(tup, "1", DefaultTarget64); !ok || got != 8 {
		t.Errorf("element 1 is at %d (%v), want 8", got, ok)
	}
	if got, ok := Offsetof(tup, "second", DefaultTarget64); !ok || got != 8 {
		t.Errorf("the labelled element is at %d (%v), want 8", got, ok)
	}
}

// TestOffsetofMatchesSizeof: laying out every field and then adding
// the last one's size gives what Sizeof says, since both are the same
// rule applied once.
func TestOffsetofMatchesSizeof(t *testing.T) {
	s := &Struct{Name: "S", Fields: []*Field{
		{Name: "a", Type: Typ[Bool]},
		{Name: "b", Type: Typ[Double]},
		{Name: "c", Type: Typ[Int16]},
	}}
	off, ok := Offsetof(s, "c", DefaultTarget64)
	if !ok {
		t.Fatal("no field c")
	}
	if got, want := off+Sizeof(Typ[Int16], DefaultTarget64), Sizeof(s, DefaultTarget64); got != want {
		t.Errorf("the last field ends at %d, and the struct is %d", got, want)
	}
}
