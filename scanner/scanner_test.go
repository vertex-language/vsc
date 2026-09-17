package scanner

import (
	"testing"

	"github.com/vertex-language/vsc/token"
)

func TestScannerComments(t *testing.T) {
	src := `// Single line comment
/* Simple block comment */
/* Nested /* block */ comment */
let x = 10`
	f := token.NewFile("comment.vs", []byte(src))
	toks, diags := Scan(f, 0)
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if len(toks) < 5 {
		t.Fatalf("expected at least 5 tokens, got %d", len(toks))
	}
	if toks[0].Kind != token.LET {
		t.Errorf("expected LET, got %v", toks[0].Kind)
	}
}

func TestScannerOperators(t *testing.T) {
	src := `x + y; +x; x+; a == b; c = d`
	f := token.NewFile("oper.vs", []byte(src))
	toks, diags := Scan(f, 0)
	if len(diags) != 0 {
		t.Fatalf("unexpected diags: %v", diags)
	}

	kinds := []token.Kind{}
	for _, tk := range toks {
		if tk.Kind != token.EOF {
			kinds = append(kinds, tk.Kind)
		}
	}
	// x + y -> IDENT, OPER_BINARY, IDENT, SEMI
	if len(kinds) < 4 || kinds[1] != token.OPER_BINARY {
		t.Errorf("expected binary operator, got %v", kinds[1])
	}
}

func TestScannerDiagnostics(t *testing.T) {
	// Test unterminated string
	src1 := `"unterminated string`
	f1 := token.NewFile("err1.vs", []byte(src1))
	_, diags1 := Scan(f1, 0)
	if len(diags1) == 0 {
		t.Errorf("expected diagnostic for unterminated string")
	}

	// Test illegal character
	src2 := "let x = \x01"
	f2 := token.NewFile("err2.vs", []byte(src2))
	_, diags2 := Scan(f2, 0)
	if len(diags2) == 0 {
		t.Errorf("expected diagnostic for control character")
	}

	// Test unterminated comment
	src3 := "/* unterminated comment"
	f3 := token.NewFile("err3.vs", []byte(src3))
	_, diags3 := Scan(f3, 0)
	if len(diags3) == 0 {
		t.Errorf("expected diagnostic for unterminated block comment")
	}
}

func TestScannerNumbers(t *testing.T) {
	src := `0b1010_0101 0o777 0xFF_EE 123_456 0x1.8p3 3.14159 1e10 2.5e-3 0x1p+4`
	f := token.NewFile("num.vs", []byte(src))
	toks, diags := Scan(f, 0)
	if len(diags) != 0 {
		t.Fatalf("unexpected diags: %v", diags)
	}
	expected := []token.Kind{
		token.INT_LIT, token.INT_LIT, token.INT_LIT, token.INT_LIT,
		token.FLOAT_LIT, token.FLOAT_LIT, token.FLOAT_LIT, token.FLOAT_LIT, token.FLOAT_LIT, token.EOF,
	}
	if len(toks) != len(expected) {
		t.Fatalf("expected %d tokens, got %d", len(expected), len(toks))
	}
	for i, exp := range expected {
		if toks[i].Kind != exp {
			t.Errorf("token %d: expected %v, got %v", i, exp, toks[i].Kind)
		}
	}
}

func TestScannerShebang(t *testing.T) {
	src := "#!/usr/bin/env swift\nlet x = 1"
	f := token.NewFile("shebang.vs", []byte(src))
	toks, diags := Scan(f, 0)
	if len(diags) != 0 {
		t.Fatalf("unexpected diags: %v", diags)
	}
	if len(toks) < 4 || toks[0].Kind != token.LET {
		t.Fatalf("expected LET after shebang, got %v", toks)
	}
}

func TestScannerEscapes(t *testing.T) {
	src := `"escapes: \0 \\ \t \n \r \" \' \u{1f600} \u{41}"`
	f := token.NewFile("escapes.vs", []byte(src))
	toks, diags := Scan(f, 0)
	if len(diags) != 0 {
		t.Fatalf("unexpected diags: %v", diags)
	}
	if len(toks) < 3 {
		t.Fatalf("expected string tokens, got %d", len(toks))
	}
}

func TestScannerInvalidEscapes(t *testing.T) {
	cases := []string{
		`"\u"`,
		`"\u{xyz}"`,
		`"\u{123"`,
	}
	for _, c := range cases {
		f := token.NewFile("invalidescape.vs", []byte(c))
		_, diags := Scan(f, 0)
		if len(diags) == 0 {
			t.Errorf("expected diagnostic for invalid escape %q", c)
		}
	}
}

func TestScannerDollarIdentifiers(t *testing.T) {
	src := `$0 $1 $arg`
	f := token.NewFile("dollar.vs", []byte(src))
	toks, diags := Scan(f, 0)
	if len(diags) != 0 {
		t.Fatalf("unexpected diags: %v", diags)
	}
	if toks[0].Kind != token.IDENT || toks[1].Kind != token.IDENT || toks[2].Kind != token.IDENT {
		t.Errorf("expected identifier tokens, got %v", toks)
	}

	// Lone dollar produces diagnostic
	fLone := token.NewFile("lone.vs", []byte("$"))
	_, diagsLone := Scan(fLone, 0)
	if len(diagsLone) == 0 {
		t.Errorf("expected diagnostic for lone dollar")
	}
}

func TestScannerBacktickIdentifiers(t *testing.T) {
	src := "`class` `default`"
	f := token.NewFile("backticks.vs", []byte(src))
	toks, diags := Scan(f, 0)
	if len(diags) != 0 {
		t.Fatalf("unexpected diags: %v", diags)
	}
	if toks[0].Kind != token.IDENT || toks[1].Kind != token.IDENT {
		t.Errorf("expected IDENT tokens for backtick names, got %v", toks)
	}

	// Unterminated backtick
	srcErr := "`unclosed"
	fErr := token.NewFile("unclosed.vs", []byte(srcErr))
	_, diagsErr := Scan(fErr, 0)
	if len(diagsErr) == 0 {
		t.Errorf("expected diagnostic for unclosed backtick")
	}
}

func TestScannerCommentsMode(t *testing.T) {
	src := "// line comment\n/* block */ let a = 1"
	f := token.NewFile("comments.vs", []byte(src))
	toks, diags := Scan(f, ScanComments)
	if len(diags) != 0 {
		t.Fatalf("unexpected diags: %v", diags)
	}
	hasComment := false
	for _, tk := range toks {
		if tk.Kind == token.COMMENT {
			hasComment = true
			break
		}
	}
	if !hasComment {
		t.Errorf("expected COMMENT token in ScanComments mode")
	}
}

func TestScannerRegex(t *testing.T) {
	src := `let r = /[a-zA-Z]+/`
	f := token.NewFile("regex.vs", []byte(src))
	toks, diags := Scan(f, 0)
	if len(diags) != 0 {
		t.Fatalf("unexpected diags: %v", diags)
	}
	hasRegex := false
	for _, tk := range toks {
		if tk.Kind == token.REGEX_LIT {
			hasRegex = true
			break
		}
	}
	if !hasRegex {
		t.Errorf("expected REGEX_LIT token")
	}
}
