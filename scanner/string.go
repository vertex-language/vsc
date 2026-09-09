package scanner

import (
	"fmt"

	"github.com/vertex-language/vsc/token"
)

// openString opens a string literal at its quote. start is the first
// byte of the literal — the first '#' of an
// ExtendedStringLiteralDelimiter, if it has one — and pounds is that
// delimiter's length, which the cursor already sits past.
//
// The pound delimiters are their own tokens, outside the quotes, so a
// consumer can tell #"a"# from "a" without measuring spans.
func (s *scanner) openString(start, pounds int) {
	fl := token.Flags(0)
	if pounds > 0 {
		s.emit(token.POUND_DELIM, start)
		fl |= token.FlagRaw
	}

	q := s.off
	multiline := s.at(0, '"') && s.at(1, '"') && s.at(2, '"')
	if multiline {
		s.off += 3
		fl |= token.FlagMultiline
		s.emitFlags(token.MULTILINE_STRING_QUOTE, q, fl)
		s.openMultilineLine(q)
	} else {
		s.off++
		s.emitFlags(token.STRING_QUOTE, q, fl)
	}
	s.strs = append(s.strs, strLit{open: start, pounds: pounds, multiline: multiline})
}

// openMultilineLine enforces the one rule the opening delimiter of a
// multiline literal carries: nothing but whitespace may follow it on
// its line. The first line of the literal's content is the next one,
// and that is what makes the closing delimiter's indentation
// meaningful.
func (s *scanner) openMultilineLine(q int) {
	for i := s.off; i < len(s.src); i++ {
		switch {
		case isNewline(s.src[i]):
			return
		case s.src[i] == ' ' || s.src[i] == '\t':
		default:
			s.report(token.Error, q, s.off,
				"a multiline string literal must begin on a new line after its opening delimiter")
			return
		}
	}
}

// scanStringBody scans the text of the innermost open literal up to
// the next thing that is not text: an interpolation, the closing
// delimiter, or the end of the line or file with neither.
//
// Segments are emitted undecoded — an EscapedCharacter keeps its
// backslash — because a segment's value depends on the literal's
// pound count and, for a multiline literal, on the indentation of the
// closing delimiter. Both are above this package.
func (s *scanner) scanStringBody(st *strLit) {
	seg := s.off
	for s.off < len(s.src) {
		c := s.src[s.off]
		switch {
		case c == '"' && s.closes(st):
			s.segment(seg)
			s.closeString(st)
			return

		case c == '\\' && s.escaped(st):
			if s.interpolates(st) {
				s.segment(seg)
				s.openInterp(st)
				return
			}
			s.scanEscape(st)

		case isNewline(c):
			if !st.multiline {
				s.segment(seg)
				s.report(token.Error, st.open, s.off, "unterminated string literal")
				s.strs = s.strs[:len(s.strs)-1]
				return
			}
			s.off++
			if c == '\r' && s.at(0, '\n') {
				s.off++
			}

		default:
			s.off++
		}
	}
	s.segment(seg) // end of file: Scan reports the unterminated literal
}

// closes reports whether the quote at the cursor is the literal's
// closing delimiter — which, for a raw literal, means the pound run
// that follows it is as long as the one that opened it.
func (s *scanner) closes(st *strLit) bool {
	n := 1
	if st.multiline {
		if !(s.at(1, '"') && s.at(2, '"')) {
			return false
		}
		n = 3
	}
	for i := 0; i < st.pounds; i++ {
		if !s.at(n+i, '#') {
			return false
		}
	}
	return true
}

// escaped reports whether the backslash at the cursor is an escape:
// in a raw literal it is one only when the literal's own pound run
// follows it, which is what makes #"a\nb"# two characters and a
// letter n.
func (s *scanner) escaped(st *strLit) bool {
	for i := 0; i < st.pounds; i++ {
		if !s.at(1+i, '#') {
			return false
		}
	}
	return true
}

// interpolates reports whether the escape at the cursor opens an
// interpolation rather than escaping a character.
func (s *scanner) interpolates(st *strLit) bool {
	return s.at(1+st.pounds, '(')
}

// segment emits the text scanned since seg, if any. An empty run
// emits nothing: every span in this front end is non-empty.
func (s *scanner) segment(seg int) {
	if s.off > seg {
		fl := token.Flags(0)
		if st := s.str(); st != nil {
			if st.multiline {
				fl |= token.FlagMultiline
			}
			if st.pounds > 0 {
				fl |= token.FlagRaw
			}
		}
		s.emitFlags(token.STRING_SEGMENT, seg, fl)
	}
}

// closeString emits the closing delimiter and pops the literal.
func (s *scanner) closeString(st *strLit) {
	if st.multiline {
		s.checkClosingLine(st)
	}
	q := s.off
	fl := token.Flags(0)
	if st.pounds > 0 {
		fl |= token.FlagRaw
	}
	if st.multiline {
		s.off += 3
		s.emitFlags(token.MULTILINE_STRING_QUOTE, q, fl|token.FlagMultiline)
	} else {
		s.off++
		s.emitFlags(token.STRING_QUOTE, q, fl)
	}
	if st.pounds > 0 {
		p := s.off
		s.off += st.pounds
		s.emit(token.POUND_DELIM, p)
	}
	s.strs = s.strs[:len(s.strs)-1]
}

// checkClosingLine enforces the other rule of a multiline literal:
// the closing delimiter begins its own line. The whitespace before it
// is the literal's indentation, and text on that line would leave
// nothing to measure it against.
func (s *scanner) checkClosingLine(st *strLit) {
	for i := s.off - 1; i >= 0; i-- {
		switch {
		case isNewline(s.src[i]):
			return
		case s.src[i] == ' ' || s.src[i] == '\t':
		default:
			s.report(token.Error, s.off, s.off+3,
				"the closing delimiter of a multiline string literal must begin on a new line")
			return
		}
	}
}

// openInterp emits the head of an interpolation — the backslash, the
// literal's pound run, and the paren — and puts the scanner back on
// the ordinary token path. The matching paren, found in scanPunct,
// brings it back here.
func (s *scanner) openInterp(st *strLit) {
	b := s.off
	s.off++
	s.emit(token.BACKSLASH, b)
	if st.pounds > 0 {
		p := s.off
		s.off += st.pounds
		s.emit(token.POUND_DELIM, p)
	}
	l := s.off
	s.off++
	s.emit(token.LPAREN, l)
	st.interp, st.depth = true, 0
}

// scanEscape consumes one EscapedCharacter or EscapedNewline. The
// value is not decoded, only checked: an escape that names nothing is
// a mistake about which characters the literal holds, and reporting
// it here is what keeps a stray backslash from swallowing the closing
// quote silently.
func (s *scanner) scanEscape(st *strLit) {
	start := s.off
	s.off += 1 + st.pounds
	if s.off >= len(s.src) {
		return
	}
	switch c := s.src[s.off]; {
	case c == '0' || c == '\\' || c == 't' || c == 'n' || c == 'r' ||
		c == '"' || c == '\'':
		s.off++

	case c == 'u':
		s.off++
		if !s.at(0, '{') {
			s.errTok(start, s.off, "expected '{' in a unicode escape sequence")
			return
		}
		s.off++
		n, v := 0, rune(0)
		for s.off < len(s.src) && isHexDigit(s.src[s.off]) {
			if n < 8 {
				v = v<<4 | rune(hexValue(s.src[s.off]))
			}
			s.off++
			n++
		}
		switch {
		case n == 0:
			s.errTok(start, s.off, "expected a hexadecimal digit in a unicode escape sequence")
		case n > 8:
			s.errTok(start, s.off, "a unicode escape sequence takes between 1 and 8 hexadecimal digits")
		case !s.at(0, '}'):
			s.errTok(start, s.off, "expected '}' to close a unicode escape sequence")
		// The escape names a scalar, and half the code points are
		// not one: a surrogate is half of a pair, and there is
		// nothing above U+10FFFF.
		case v > 0x10FFFF || (v >= 0xD800 && v <= 0xDFFF):
			s.off++
			s.errTok(start, s.off, "invalid unicode scalar")
		default:
			s.off++
		}

	case isNewline(c):
		// EscapedNewline: a line continuation, and only a multiline
		// literal has lines to continue.
		if !st.multiline {
			s.errTok(start, s.off, "an escaped newline is only valid in a multiline string literal")
			return
		}
		s.off++
		if c == '\r' && s.at(0, '\n') {
			s.off++
		}

	default:
		s.off++
		s.errTok(start, s.off, fmt.Sprintf("invalid escape sequence '\\%c'", c))
	}
}

// ---- regex literals ----

// tryRegex attempts to read a RegexLiteral at start, whose pound
// delimiter is pounds long and whose slash sits at start+pounds. It
// reports whether it read one; on failure the cursor is where it
// found it, and the slash goes on to be an operator or a comment.
//
// The extended form is unambiguous — #/…/# can be nothing else. The
// bare form shares its delimiter with division and with both comment
// openers, so it is admitted only under SE-0354's restrictions: it
// appears where an operand may begin (the slash is not bound on the
// left), neither delimiter touches whitespace, and the whole literal
// is on one line. `a / b`, `a/b` and `1 / 2` are division; `let r =
// /[a-z]+/` is a literal.
func (s *scanner) tryRegex(start, pounds int) bool {
	p := start + pounds
	if p+1 >= len(s.src) || s.src[p] != '/' {
		return false
	}
	if pounds == 0 {
		if s.leftBound(start) {
			return false // an operand ends here: this slash divides
		}
		switch c := s.src[p+1]; {
		case c == ' ' || c == '\t' || isNewline(c) || c == '/' || c == '*' || c == 0:
			return false // a comment, or a delimiter against whitespace
		}
	}

	i := p + 1
	klass := false // inside a [...] character class, where / is ordinary
	for i < len(s.src) {
		switch c := s.src[i]; {
		case c == '\\':
			i += 2
			continue
		case isNewline(c):
			if pounds == 0 {
				return false // a bare literal lives on one line
			}
		case c == '[':
			klass = true
		case c == ']':
			klass = false
		case c == '/' && !klass:
			if s.poundsFollow(i+1, pounds) {
				if pounds == 0 && (s.src[i-1] == ' ' || s.src[i-1] == '\t') {
					return false // the closing delimiter may not touch whitespace
				}
				s.off = i + 1 + pounds
				fl := token.Flags(0)
				if pounds > 0 {
					fl |= token.FlagRaw
				}
				s.emitFlags(token.REGEX_LIT, start, fl)
				return true
			}
		}
		i++
	}

	if pounds > 0 { // #/ can be nothing else, so say what went wrong
		s.off = len(s.src)
		s.errTok(start, s.off, "unterminated regex literal")
		s.emit(token.ILLEGAL, start)
		return true
	}
	return false
}

// poundsFollow reports whether n '#' characters sit at i.
func (s *scanner) poundsFollow(i, n int) bool {
	if i+n > len(s.src) {
		return false
	}
	for j := 0; j < n; j++ {
		if s.src[i+j] != '#' {
			return false
		}
	}
	return true
}
