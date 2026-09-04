package analyzer

import (
	"math"
	"testing"

	"github.com/vertex-language/vsc/ast"
)

// literalValue checks `let x = <src>` and returns the value recorded
// for the literal in it.
func literalValue(t *testing.T, src string) (Value, []string) {
	t.Helper()
	file, _ := parseSnippet(t, "let value = "+src+"\n")
	info, diags := Check([]*ast.File{file})
	msgs := make([]string, len(diags))
	for i, d := range diags {
		msgs[i] = d.Message
	}
	var out Value
	ast.Inspect(file, func(n ast.Node) bool {
		switch n.(type) {
		case *ast.BasicLit, *ast.StringLit:
			if v, ok := info.Values[n]; ok && !out.IsValid() {
				out = v
			}
		}
		return true
	})
	return out, msgs
}

// TestIntegerAndFloatValues decodes the numeric literals. Every
// expectation was printed by a Swift program holding the same
// literal — the integers as themselves, the doubles as their bit
// patterns, so that nothing is lost on the way out.
func TestIntegerAndFloatValues(t *testing.T) {
	ints := []struct {
		src  string
		want uint64
	}{
		{"42", 42},
		{"1_000_000", 1000000},
		{"0b1010_1010", 170},
		{"0o777", 511},
		{"0xFF", 255},
		{"0xdeadBEEF", 3735928559},
		{"0", 0},
		{"9223372036854775807", 9223372036854775807},
	}
	for _, c := range ints {
		v, msgs := literalValue(t, c.src)
		if len(msgs) != 0 {
			t.Errorf("%s: %v", c.src, msgs)
		}
		if v.Kind != IntValue || v.Int != c.want {
			t.Errorf("%s decoded to %v/%d, Swift says %d", c.src, v.Kind, v.Int, c.want)
		}
	}

	floats := []struct {
		src  string
		bits uint64
	}{
		{"1.0", 0x3ff0000000000000},
		{"1.5e-3", 0x3f589374bc6a7efa},
		{"0x1p-2", 0x3fd0000000000000},
		{"0x1.8p3", 0x4028000000000000},
		{"1e10", 0x4202a05f20000000},
		{"3.14159", 0x400921f9f01b866e},
		{"1_000.5", 0x408f440000000000},
		{"1E3", 0x408f400000000000},
	}
	for _, c := range floats {
		v, msgs := literalValue(t, c.src)
		if len(msgs) != 0 {
			t.Errorf("%s: %v", c.src, msgs)
		}
		if v.Kind != FloatValue || math.Float64bits(v.Float) != c.bits {
			t.Errorf("%s decoded to %v/%016x, Swift says %016x",
				c.src, v.Kind, math.Float64bits(v.Float), c.bits)
		}
	}

	// A literal too large to store is reported, not truncated.
	v, msgs := literalValue(t, "99999999999999999999999")
	if v.IsValid() || len(msgs) == 0 {
		t.Errorf("expected an overflow diagnostic, got %v / %v", v, msgs)
	}
}

// TestStringValues decodes the string literals, escapes, pound
// delimiters and multiline indentation included. Every expectation is
// what a Swift program printed for the same literal, byte for byte.
func TestStringValues(t *testing.T) {
	cases := []struct {
		name, src, want string
	}{
		{"plain", `"hello"`, "hello"},
		{"escapes", `"a\tb\nc\rd\0e\\f\"g"`, "a\tb\nc\rd\x00e\\f\"g"},
		{"scalars", `"\u{1F600}\u{41}\u{7}"`, "\U0001F600A\a"},
		{"raw", `#"no \n escape"#`, `no \n escape`},
		{"raw escape", `#"one \#n newline"#`, "one \n newline"},
		{"two pounds", `##"two ## pounds \##t tab"##`, "two ## pounds \t tab"},
		{"empty", `""`, ""},
		{"apostrophe", `"it's"`, "it's"},

		// The closing delimiter's indentation is what is stripped,
		// from every line; the first and last line breaks are not
		// part of the value.
		{"multiline", "\"\"\"\n    hello\n    world\n    \"\"\"", "hello\nworld"},
		{"deeper lines", "\"\"\"\n        keep\n          this\n        \"\"\"", "keep\n  this"},
		{"blank line", "\"\"\"\n    a\n\n    b\n    \"\"\"", "a\n\nb"},
		{"continuation", "\"\"\"\n    joined \\\n    together\n    \"\"\"", "joined together"},
		{"escape inside", "\"\"\"\n    tab\\there\n    \"\"\"", "tab\there"},
		{"one line", "\"\"\"\n    single\n    \"\"\"", "single"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v, msgs := literalValue(t, c.src)
			if len(msgs) != 0 {
				t.Errorf("%v", msgs)
			}
			if v.Kind != StringValue || v.Str != c.want {
				t.Errorf("decoded to %q, Swift says %q", v.Str, c.want)
			}
		})
	}
}

// TestInterpolatedStringValues covers the literal that has no single
// value: its runs of text are decoded and recorded one by one, and
// the literal itself carries none.
func TestInterpolatedStringValues(t *testing.T) {
	file, _ := parseSnippet(t, "let b = 2\nlet a = \"x\\ty \\(b) z\"\n")
	info, diags := Check([]*ast.File{file})
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	var texts []string
	var litHasValue bool
	ast.Inspect(file, func(n ast.Node) bool {
		switch n.(type) {
		case *ast.StringText:
			if v, ok := info.Values[n]; ok {
				texts = append(texts, v.Str)
			}
		case *ast.StringLit:
			_, litHasValue = info.Values[n]
		}
		return true
	})
	if litHasValue {
		t.Errorf("an interpolated literal should carry no value of its own")
	}
	want := []string{"x\ty ", " z"}
	if len(texts) != len(want) || texts[0] != want[0] || texts[1] != want[1] {
		t.Errorf("text runs decoded to %q, want %q", texts, want)
	}
}
