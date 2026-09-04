package scanner

import (
	"testing"

	"github.com/vertex-language/vsc/token"
)

// scanOne scans src and returns its first token and whether the scan
// was clean.
func scanOne(t *testing.T, src string) (token.Token, bool) {
	t.Helper()
	f := token.NewFile("ident.swift", []byte(src))
	toks, diags := Scan(f, 0)
	return toks[0], len(diags) == 0
}

// TestIdentCodePoints checks the boundaries of Swift's identifier
// ranges. Swift does not classify these by Unicode category — an
// emoji is an identifier character and a © is not, though both are
// symbols — so each of these was read back from swiftc rather than
// derived.
func TestIdentCodePoints(t *testing.T) {
	cases := []struct {
		r          rune
		head, cont bool
	}{
		{0x00A8, true, true}, {0x00A9, false, false}, {0x00AA, true, true},
		{0x00B1, false, false}, {0x00B2, true, true}, {0x00B6, false, false},
		{0x00B7, true, true}, {0x00BF, false, false}, {0x00C0, true, true},
		{0x00D7, false, false}, {0x167F, true, true}, {0x1680, false, false},
		{0x1681, true, true}, {0x180E, false, false}, {0x180F, true, true},
		{0x2000, false, false}, {0x200B, true, true}, {0x200E, false, false},
		{0x2040, true, true}, {0x2041, false, false}, {0x2054, true, true},
		{0x2055, false, false}, {0x2060, true, true}, {0x218F, true, true},
		{0x2190, false, false}, {0x2460, true, true}, {0x2500, false, false},
		{0x2793, true, true}, {0x2794, false, false}, {0x2DFF, true, true},
		{0x2E00, false, false}, {0x2FFF, true, true}, {0x3000, false, false},
		{0x3007, true, true}, {0x3008, false, false}, {0x302F, true, true},
		{0x3030, false, false}, {0xD7FF, true, true}, {0xFD3D, true, true},
		{0xFD3E, false, false}, {0xFDCF, true, true}, {0xFDD0, false, false},
		{0xFE1F, true, true},
		// The combining marks: a name may carry one but not open with it.
		{0x0300, false, true}, {0x036F, false, true}, {0x1DC0, false, true},
		{0x20D0, false, true}, {0xFE20, false, true}, {0xFE2F, false, true},
		{0xFE44, true, true}, {0xFE45, false, false}, {0xFE47, true, true},
		{0xFFF8, true, true}, {0xFFF9, false, false}, {0xFFFD, false, false},
		// Everything above the BMP that Swift admits, emoji included.
		{0x10000, true, true}, {0x1F600, true, true}, {0x1FFFD, true, true},
		{0x1FFFE, false, false}, {0x20000, true, true}, {0xE0000, true, true},
		{0xEFFFD, true, true}, {0xF0000, false, false},
	}
	for _, c := range cases {
		if got := isIdentHead(c.r); got != c.head {
			t.Errorf("isIdentHead(U+%04X) = %v, want %v", c.r, got, c.head)
		}
		if got := isIdentChar(c.r); got != c.cont {
			t.Errorf("isIdentChar(U+%04X) = %v, want %v", c.r, got, c.cont)
		}
	}

	// The whole run, not just the classification.
	for _, src := range []string{"😀", "café", "漢字", "x😀y", "aٍ"} {
		tok, ok := scanOne(t, src+" = 1")
		if !ok || tok.Kind != token.IDENT {
			t.Errorf("%q: got %v, clean=%v; want a clean IDENT", src, tok.Kind, ok)
		}
	}
	for _, src := range []string{"©", "→", "±"} {
		if tok, _ := scanOne(t, src+" = 1"); tok.Kind == token.IDENT {
			t.Errorf("%q scanned as an identifier", src)
		}
	}
}

// TestRawIdentifiers covers the backtick's two jobs: escaping a
// keyword, and naming something that is not an identifier at all.
func TestRawIdentifiers(t *testing.T) {
	good := []string{
		"`class`", "`if`", "`hello world`", "`f(x)`", "`123`",
		"`emoji 😀 here`", "`a+b`", "` a `", "`a.b`", "`$`",
	}
	for _, src := range good {
		tok, ok := scanOne(t, src+" = 1")
		if !ok || tok.Kind != token.IDENT || !tok.Flags.Has(token.FlagEscaped) {
			t.Errorf("%s: got %v flags=%b clean=%v; want a clean escaped IDENT",
				src, tok.Kind, tok.Flags, ok)
		}
	}
	// Empty, whitespace-only, all-operator, and the characters a raw
	// identifier may not hold.
	bad := []string{
		"``", "` `", "`  `", "`..`", "`+`", "`←→`", "`a\\b`", "`a\tb`",
		"`a\nb`", "`unterminated",
	}
	for _, src := range bad {
		if tok, ok := scanOne(t, src+" = 1"); ok && tok.Kind == token.IDENT {
			t.Errorf("%q scanned as a valid identifier", src)
		}
	}
}
