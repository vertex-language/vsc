package scanner

import "github.com/vertex-language/vsc/token"

// scanIdent consumes an IdentifierHead {IdentifierCharacter} run and
// looks it up: a reserved word becomes its own kind, everything else
// is an IDENT. The contextual words — get, set, some, async, weak and
// the rest — stay IDENT here; matching them by spelling is the
// parser's job, and only in the positions that give them meaning.
func (s *scanner) scanIdent(start int) {
	for s.off < len(s.src) {
		r, w := s.rune()
		if !isIdentChar(r) {
			break
		}
		s.off += w
	}
	s.emit(token.Lookup(string(s.src[start:s.off])), start)
}

// scanEscapedIdent consumes `identifier`. The backticks are part of
// the span but not of the name: Ident.Text strips them.
//
// A backtick does two jobs. It escapes a spelling that is otherwise a
// keyword, which is what lets a program declare something called
// `class`; and it introduces a raw identifier, whose name need not be
// an identifier at all — `hello world` and `f(x)` are names a test
// framework gives its cases.
//
// What a raw identifier may not hold: a backslash, a line break, or
// any whitespace but the space itself. What it may not be: empty,
// all spaces, or a name made only of operator characters, which would
// have no way to be told from an operator at the use site.
func (s *scanner) scanEscapedIdent() {
	start := s.off
	s.off++ // the opening backtick

	body := s.off
	valid, allSpace, allOper := true, true, true
	for s.off < len(s.src) {
		r, w := s.rune()
		if r == '`' {
			break
		}
		if r == '\\' || isNewline(byte(r)) || (isSpaceRune(r) && r != ' ') {
			valid = false
			break
		}
		if r != ' ' {
			allSpace = false
		}
		if !(r < utf8Self && isOperHead(byte(r)) || r == '.') && !isOperChar(r) {
			allOper = false
		}
		s.off += w
	}

	switch {
	case !valid || s.off == body || !s.at(0, '`'):
		if s.off == body && s.at(0, '`') {
			s.off++
		}
		s.errTok(start, s.off, "expected an identifier after '`'")
	case allSpace || allOper:
		s.off++
		s.errTok(start, s.off, "a raw identifier must hold a name")
	default:
		s.off++
		s.emitFlags(token.IDENT, start, token.FlagEscaped)
		return
	}
	s.emit(token.ILLEGAL, start)
}

// isSpaceRune reports whether r is whitespace or a control character
// — what a raw identifier may not hold, the space itself excepted.
func isSpaceRune(r rune) bool {
	switch {
	case r <= 0x20, r == 0x7F:
		return true
	case r == 0x85, r == 0xA0, r == 0x1680,
		r >= 0x2000 && r <= 0x200A,
		r == 0x2028, r == 0x2029, r == 0x202F, r == 0x205F, r == 0x3000:
		return true
	}
	return false
}

// scanDollarIdent consumes the two $ forms of Identifier: the
// implicit parameter name $0, and the property wrapper projection
// $value. They differ in nothing the scanner can act on, so both are
// IDENT and the analyzer reads the spelling.
func (s *scanner) scanDollarIdent() {
	start := s.off
	s.off++ // the '$'

	body := s.off
	for s.off < len(s.src) {
		r, w := s.rune()
		if !isIdentChar(r) {
			break
		}
		s.off += w
	}
	if s.off == body {
		s.errTok(start, s.off, "expected a name or a number after '$'")
		s.emit(token.ILLEGAL, start)
		return
	}
	s.emit(token.IDENT, start)
}

// The identifier code points. Swift does not classify them by
// Unicode category: it names ranges, and the set is not the letters —
// `let 😀 = 1` is a Swift program, and U+00A9 © is not an identifier
// character though it is a symbol like the emoji is. These are the
// ranges Swift's own lexer holds, in its order.
var identHeadRanges = [...][2]rune{
	{0x00A8, 0x00A8}, {0x00AA, 0x00AA}, {0x00AD, 0x00AD}, {0x00AF, 0x00AF},
	{0x00B2, 0x00B5}, {0x00B7, 0x00BA}, {0x00BC, 0x00BE},
	{0x00C0, 0x00D6}, {0x00D8, 0x00F6}, {0x00F8, 0x00FF},
	{0x0100, 0x02FF}, {0x0370, 0x167F}, {0x1681, 0x180D},
	{0x180F, 0x1DBF}, {0x1E00, 0x1FFF},
	{0x200B, 0x200D}, {0x202A, 0x202E}, {0x203F, 0x2040}, {0x2054, 0x2054},
	{0x2060, 0x20CF}, {0x2100, 0x218F}, {0x2460, 0x24FF}, {0x2776, 0x2793},
	{0x2C00, 0x2DFF}, {0x2E80, 0x2FFF},
	{0x3004, 0x3007}, {0x3021, 0x302F}, {0x3031, 0x303F},
	{0x3040, 0xD7FF},
	{0xF900, 0xFD3D}, {0xFD40, 0xFDCF}, {0xFDF0, 0xFE1F},
	{0xFE30, 0xFE44}, {0xFE47, 0xFFF8},
	{0x10000, 0x1FFFD}, {0x20000, 0x2FFFD}, {0x30000, 0x3FFFD},
	{0x40000, 0x4FFFD}, {0x50000, 0x5FFFD}, {0x60000, 0x6FFFD},
	{0x70000, 0x7FFFD}, {0x80000, 0x8FFFD}, {0x90000, 0x9FFFD},
	{0xA0000, 0xAFFFD}, {0xB0000, 0xBFFFD}, {0xC0000, 0xCFFFD},
	{0xD0000, 0xDFFFD}, {0xE0000, 0xEFFFD},
}

// The combining marks an identifier may carry but not open with.
var identContRanges = [...][2]rune{
	{0x0300, 0x036F}, {0x1DC0, 0x1DFF}, {0x20D0, 0x20FF}, {0xFE20, 0xFE2F},
}

func inRanges(r rune, ranges [][2]rune) bool {
	lo, hi := 0, len(ranges)-1
	for lo <= hi {
		m := int(uint(lo+hi) >> 1)
		switch {
		case r < ranges[m][0]:
			hi = m - 1
		case r > ranges[m][1]:
			lo = m + 1
		default:
			return true
		}
	}
	return false
}

// isIdentHead is IdentifierHead: the ASCII letters, '_', and the
// scalars in identHeadRanges.
func isIdentHead(r rune) bool {
	if r < utf8Self {
		return r == '_' || 'a' <= r && r <= 'z' || 'A' <= r && r <= 'Z'
	}
	return inRanges(r, identHeadRanges[:])
}

// isIdentChar is IdentifierCharacter: an IdentifierHead, a decimal
// digit, or a combining mark.
func isIdentChar(r rune) bool {
	if r < utf8Self {
		return r == '_' || 'a' <= r && r <= 'z' || 'A' <= r && r <= 'Z' || '0' <= r && r <= '9'
	}
	return inRanges(r, identHeadRanges[:]) || inRanges(r, identContRanges[:])
}

const utf8Self = 0x80

// isOperChar is the non-ASCII half of OperatorCharacter: the scalars
// reserved for operators, and the combining marks that may follow
// one. The grammar leaves the sets to a narrative phrase; these are
// the ranges Swift reserves.
func isOperChar(r rune) bool {
	return isOperHeadRune(r) || isOperCombiningRune(r)
}

// isOperHeadRune reports whether r is a non-ASCII OperatorHead.
func isOperHeadRune(r rune) bool {
	switch {
	case r >= 0x00A1 && r <= 0x00A7,
		r == 0x00A9, r == 0x00AB, r == 0x00AC, r == 0x00AE,
		r == 0x00B0, r == 0x00B1, r == 0x00B6, r == 0x00BB, r == 0x00BF,
		r == 0x00D7, r == 0x00F7,
		r >= 0x2016 && r <= 0x2017,
		r >= 0x2020 && r <= 0x2027,
		r >= 0x2030 && r <= 0x203E,
		r >= 0x2041 && r <= 0x2053,
		r >= 0x2055 && r <= 0x205E,
		r >= 0x2190 && r <= 0x23FF,
		r >= 0x2500 && r <= 0x2775,
		r >= 0x2794 && r <= 0x2BFF,
		r >= 0x2E00 && r <= 0x2E7F,
		r >= 0x3001 && r <= 0x3003,
		r >= 0x3008 && r <= 0x3020,
		r == 0x3030:
		return true
	}
	return false
}

// isOperCombiningRune reports whether r is a combining scalar that an
// operator may carry, but not open with.
func isOperCombiningRune(r rune) bool {
	switch {
	case r >= 0x0300 && r <= 0x036F,
		r >= 0x1DC0 && r <= 0x1DFF,
		r >= 0x20D0 && r <= 0x20FF,
		r >= 0xFE00 && r <= 0xFE0F,
		r >= 0xFE20 && r <= 0xFE2F,
		r >= 0xE0100 && r <= 0xE01EF:
		return true
	}
	return false
}
