package token

import (
	"strings"
	"testing"
)

func TestFilePositions(t *testing.T) {
	src := []byte("hello\nworld\r\nvertex")
	f := NewFile("sample.vs", src)

	if f.Name() != "sample.vs" {
		t.Errorf("expected sample.vs, got %s", f.Name())
	}
	if f.Size() != len(src) {
		t.Errorf("expected size %d, got %d", len(src), f.Size())
	}
	if f.LineCount() != 3 {
		t.Errorf("expected 3 lines, got %d", f.LineCount())
	}

	// First line: "hello"
	p0 := f.Pos(0)
	pos0 := f.Position(p0)
	if pos0.Line != 1 || pos0.Column != 1 {
		t.Errorf("pos0: expected 1:1, got %d:%d", pos0.Line, pos0.Column)
	}
	if string(f.LineText(1)) != "hello" {
		t.Errorf("expected line 1 'hello', got %q", f.LineText(1))
	}

	// Second line: "world"
	p1 := f.Pos(6)
	pos1 := f.Position(p1)
	if pos1.Line != 2 || pos1.Column != 1 {
		t.Errorf("pos1: expected 2:1, got %d:%d", pos1.Line, pos1.Column)
	}
	if string(f.LineText(2)) != "world" {
		t.Errorf("expected line 2 'world', got %q", f.LineText(2))
	}

	// Third line: "vertex"
	p2 := f.Pos(13)
	pos2 := f.Position(p2)
	if pos2.Line != 3 || pos2.Column != 1 {
		t.Errorf("pos2: expected 3:1, got %d:%d", pos2.Line, pos2.Column)
	}
	if string(f.LineText(3)) != "vertex" {
		t.Errorf("expected line 3 'vertex', got %q", f.LineText(3))
	}

	// LineText edge cases: line 0, line 4 (out of bounds)
	if f.LineText(0) != nil {
		t.Errorf("expected nil for line 0")
	}
	if f.LineText(4) != nil {
		t.Errorf("expected nil for line 4")
	}

	// Slice check
	slice := f.Slice(f.Pos(6), f.Pos(11))
	if string(slice) != "world" {
		t.Errorf("expected slice 'world', got %q", slice)
	}

	// Position for NoPos panics
	assertPanic(t, "Position(NoPos)", func() { f.Position(NoPos) })

	// Test bounds panics
	assertPanic(t, "Pos(-1)", func() { f.Pos(-1) })
	assertPanic(t, "Pos(len+1)", func() { f.Pos(len(src) + 1) })
	assertPanic(t, "Offset(0)", func() { f.Offset(Pos(0)) })
	assertPanic(t, "Offset(too large)", func() { f.Offset(Pos(999999)) })
}

func assertPanic(t *testing.T, name string, fn func()) {
	t.Helper()
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("%s did not panic", name)
		}
	}()
	fn()
}

func TestKeywords(t *testing.T) {
	cases := []struct {
		word string
		kind Kind
	}{
		{"func", FUNC},
		{"class", CLASS},
		{"struct", STRUCT},
		{"enum", ENUM},
		{"protocol", PROTOCOL},
		{"actor", IDENT}, // contextual
		{"each", IDENT},  // contextual
		{"some", IDENT},  // contextual
		{"any", IDENT},   // contextual
		{"true", TRUE},
		{"false", FALSE},
		{"nil", NIL},
		{"try", TRY},
		{"repeat", REPEAT},
		{"foobar", IDENT},
	}
	for _, tc := range cases {
		k := Lookup(tc.word)
		if k != tc.kind {
			t.Errorf("Lookup(%q): expected %v, got %v", tc.word, tc.kind, k)
		}
	}

	if !IsContextualKeyword("each") {
		t.Errorf("expected 'each' to be contextual keyword")
	}
	if !IsContextualKeyword("actor") {
		t.Errorf("expected 'actor' to be contextual keyword")
	}
	if !IsContextualKeyword("discard") {
		t.Errorf("expected 'discard' to be contextual keyword")
	}
	if IsContextualKeyword("foobar") {
		t.Errorf("'foobar' should not be contextual keyword")
	}
}

// TestTablesAgree holds the two keyword tables to their definitions:
// a contextual word is one a program may still use as a name, so no
// spelling may be in both, and every reserved word and # word has a
// spelling to be looked up by.
func TestTablesAgree(t *testing.T) {
	for word := range contextual {
		if k := Lookup(word); k != IDENT {
			t.Errorf("%q is contextual and reserved as %v: a program cannot name anything after it", word, k)
		}
	}
	for k := keyword_beg + 1; k < keyword_end; k++ {
		if names[k] == "" {
			t.Errorf("keyword %d has no spelling", k)
			continue
		}
		if got := Lookup(names[k]); got != k {
			t.Errorf("Lookup(%q) = %v, want %v", names[k], got, k)
		}
	}
	for k := pound_beg + 1; k < pound_end; k++ {
		if names[k] == "" {
			t.Errorf("pound keyword %d has no spelling", k)
			continue
		}
		if got := LookupPound(names[k][1:]); got != k {
			t.Errorf("LookupPound(%q) = %v, want %v", names[k][1:], got, k)
		}
	}
	for k := Kind(0); k <= Kind(pound_end); k++ {
		switch k {
		case literal_beg, literal_end, punct_beg, punct_end,
			oper_beg, oper_end, keyword_beg, keyword_end, pound_beg, pound_end:
			continue
		}
		if names[k] == "" {
			t.Errorf("kind %d has no name", k)
		}
	}
}

func TestPoundKeywords(t *testing.T) {
	if LookupPound("if") != POUND_IF {
		t.Errorf("expected POUND_IF")
	}
	if LookupPound("available") != POUND_AVAILABLE {
		t.Errorf("expected POUND_AVAILABLE")
	}
	if LookupPound("customMacro") != POUND {
		t.Errorf("expected POUND for custom macro")
	}
}

func TestKindPredicates(t *testing.T) {
	if !INT_LIT.IsLiteral() {
		t.Errorf("INT_LIT should be literal")
	}
	if !OPER_PREFIX.IsOperator() {
		t.Errorf("OPER_PREFIX should be operator")
	}
	if !LPAREN.IsPunct() {
		t.Errorf("LPAREN should be punct")
	}
	if !FUNC.IsKeyword() {
		t.Errorf("FUNC should be keyword")
	}
	if !POUND_IF.IsPound() {
		t.Errorf("POUND_IF should be pound")
	}

	// Unknown kind String representation (exercises itoa fallback)
	unknownKind := Kind(250)
	str := unknownKind.String()
	if str != "Kind(250)" {
		t.Errorf("expected 'Kind(250)', got %q", str)
	}
}

func TestDiagnosticPrint(t *testing.T) {
	src := []byte("let x = 123\nlet y = ")
	f := NewFile("diag.vs", src)
	d := Diagnostic{
		Pos:      f.Pos(len(src)),
		End:      f.Pos(len(src)),
		Severity: Error,
		Message:  "expected expression",
	}
	out := d.Print(f)
	if !strings.Contains(out, "diag.vs:2:9: error: expected expression") {
		t.Errorf("unexpected diagnostic output: %s", out)
	}
}

func TestTokenHelpers(t *testing.T) {
	// Flags
	flags := FlagAdjacent | FlagNLBefore
	if !flags.Has(FlagAdjacent) {
		t.Errorf("expected FlagAdjacent")
	}
	if flags.Has(FlagMultiline) {
		t.Errorf("unexpected FlagMultiline")
	}

	// Kind string
	if FUNC.String() != "func" {
		t.Errorf("expected 'func', got %s", FUNC.String())
	}

	// File helpers
	src := []byte("first\nsecond")
	f := NewFile("f.vs", src)
	if string(f.Text()) != string(src) {
		t.Errorf("expected Text %q, got %q", src, f.Text())
	}
	if f.Line(f.Pos(0)) != 1 || f.Line(f.Pos(6)) != 2 {
		t.Errorf("Line lookup mismatch")
	}
	between := f.Between(Token{End: f.Pos(5)}, Token{Pos: f.Pos(6)})
	if string(between) != "\n" {
		t.Errorf("expected between to be '\\n', got %q", between)
	}

	// Between edge case: inverted pos (lo > hi clamps to empty slice)
	inverted := f.Between(Token{End: f.Pos(6)}, Token{Pos: f.Pos(5)})
	if len(inverted) != 0 {
		t.Errorf("expected empty slice when prev.End > next.Pos, got %q", inverted)
	}

	// Position String
	pos := f.Position(f.Pos(0))
	if pos.String() != "f.vs:1:1" {
		t.Errorf("expected 'f.vs:1:1', got %s", pos.String())
	}

	// Diagnostics sort and string
	diags := []Diagnostic{
		{Pos: f.Pos(6), End: f.Pos(7), Message: "b_second"},
		{Pos: f.Pos(6), End: f.Pos(8), Message: "a_longer_end"},
		{Pos: f.Pos(6), End: f.Pos(7), Message: "a_first"},
		{Pos: f.Pos(0), End: f.Pos(1), Message: "initial"},
	}
	SortDiagnostics(diags)
	if diags[0].Message != "initial" || diags[1].Message != "a_first" || diags[2].Message != "b_second" || diags[3].Message != "a_longer_end" {
		t.Errorf("SortDiagnostics failed: %v", diags)
	}
	if Error.String() != "error" || Warn.String() != "warning" || Note.String() != "note" || Severity(99).String() != "severity(99)" {
		t.Errorf("Severity.String() failed, got %q", Severity(99).String())
	}
}
