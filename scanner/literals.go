package scanner

import (
	"fmt"

	"github.com/vertex-language/vsc/token"
)

// scanNumber consumes one IntegerLiteral or FloatingPointLiteral and
// classifies it. The value is a decoding concern, phases above this
// one: the token keeps the spelling, underscores and all.
//
// The sign is not part of the token. NumericLiteral admits a leading
// '-', but only spacing can tell `-1` from `a -1`, and spacing is
// what the operator classifier already reads — so a negative literal
// arrives as OPER_PREFIX '-' over the literal, and the tree the
// parser builds says so.
func (s *scanner) scanNumber() {
	start := s.off

	// The prefixed bases. Uppercase 0B, 0O and 0X are not spellings
	// of them: they scan as a 0 with an identifier stuck to it, and
	// the trailing-character check below is what reports that.
	if s.src[s.off] == '0' && s.off+1 < len(s.src) {
		switch s.src[s.off+1] {
		case 'b':
			s.off += 2
			if !s.digitRun(isBinDigit) {
				s.errTok(start, s.off, "expected a digit after the binary literal prefix '0b'")
			}
			s.finishNumber(start, token.INT_LIT, "binary")
			return
		case 'o':
			s.off += 2
			if !s.digitRun(isOctDigit) {
				s.errTok(start, s.off, "expected a digit after the octal literal prefix '0o'")
			}
			s.finishNumber(start, token.INT_LIT, "octal")
			return
		case 'x':
			s.off += 2
			s.scanHexNumber(start)
			return
		}
	}

	kind := token.INT_LIT
	s.digitRun(isDigit)

	// A DecimalFraction only where a digit follows the point: 1.max
	// is a member access on 1, and 1..<2 is a range.
	if s.at(0, '.') && s.off+1 < len(s.src) && isDigit(s.src[s.off+1]) {
		s.off++
		s.digitRun(isDigit)
		kind = token.FLOAT_LIT
	}
	if s.at(0, 'e') || s.at(0, 'E') {
		if s.scanExponent(start, "decimal") {
			kind = token.FLOAT_LIT
		}
	}
	s.finishNumber(start, kind, "decimal")
}

// scanHexNumber consumes the rest of a 0x literal. A
// HexadecimalFraction is only ever part of a float, and the grammar
// requires the binary exponent that says where the point goes: 0x1.8
// is not a number, 0x1.8p0 is.
func (s *scanner) scanHexNumber(start int) {
	kind := token.INT_LIT
	if !s.digitRun(isHexDigit) {
		s.errTok(start, s.off, "expected a digit after the hexadecimal literal prefix '0x'")
	}
	if s.at(0, '.') && s.off+1 < len(s.src) && isHexDigit(s.src[s.off+1]) {
		s.off++
		s.digitRun(isHexDigit)
		kind = token.FLOAT_LIT
	}
	if s.at(0, 'p') || s.at(0, 'P') {
		s.scanExponent(start, "decimal")
		kind = token.FLOAT_LIT
	} else if kind == token.FLOAT_LIT {
		s.errTok(start, s.off, "hexadecimal floating point literal requires a 'p' exponent")
	}
	s.finishNumber(start, kind, "hexadecimal")
}

// scanExponent consumes FloatingPointE/P [Sign] DecimalLiteral. It
// reports whether one was there: `1e` with nothing after it is a
// malformed exponent, but `0x1e` is a hexadecimal digit and never
// reaches here.
func (s *scanner) scanExponent(start int, digits string) bool {
	mark := s.off
	s.off++ // 'e', 'E', 'p' or 'P'
	if s.at(0, '+') || s.at(0, '-') {
		s.off++
	}
	if !s.digitRun(isDigit) {
		s.off = mark
		s.errTok(start, s.off, fmt.Sprintf("expected a %s digit in the floating point exponent", digits))
		return false
	}
	return true
}

// digitRun consumes DecimalDigit {DecimalLiteralCharacter} and its
// siblings: one digit of the base, then any run of digits and
// underscores. A leading underscore is not part of a literal — _1 is
// an identifier — so the first character must be a digit.
func (s *scanner) digitRun(ok func(byte) bool) bool {
	if s.off >= len(s.src) || !ok(s.src[s.off]) {
		return false
	}
	for s.off < len(s.src) && (ok(s.src[s.off]) || s.src[s.off] == '_') {
		s.off++
	}
	return true
}

// finishNumber emits the literal, first consuming any identifier
// characters stuck to its end — 0b12, 1_000km, 0X1 — so the mistake
// is one token and one diagnostic rather than a literal followed by a
// name nothing declared.
func (s *scanner) finishNumber(start int, kind token.Kind, base string) {
	if s.off < len(s.src) {
		if r, _ := s.rune(); isIdentHead(r) {
			bad := s.off
			for s.off < len(s.src) {
				r, w := s.rune()
				if !isIdentChar(r) {
					break
				}
				s.off += w
			}
			s.errTok(bad, s.off, fmt.Sprintf("%q is not a valid digit in a %s literal",
				string(s.src[bad:s.off]), base))
		}
	}
	s.emit(kind, start)
}
