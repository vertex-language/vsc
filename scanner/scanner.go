package scanner

import (
	"fmt"
	"unicode/utf8"

	"github.com/vertex-language/vsc/token"
)

// Mode controls optional scanner behavior.
type Mode uint

const (
	// ScanComments keeps COMMENT tokens in the stream; without it
	// they are trivia, still reachable via token.File.Between.
	ScanComments Mode = 1 << iota
)

// Scan is the entire API. The slice always ends in an EOF token (the
// one zero-width span). Diagnostics are sorted.
func Scan(f *token.File, mode Mode) ([]token.Token, []token.Diagnostic) {
	s := &scanner{f: f, src: f.Text(), mode: mode, nlBefore: true}
	s.skipShebang()
	for {
		if s.off >= len(s.src) {
			break
		}
		if st := s.str(); st != nil && !st.interp {
			s.scanStringBody(st)
			continue
		}
		s.skipTrivia()
		if s.off >= len(s.src) {
			break
		}
		s.scanToken()
	}
	for len(s.strs) > 0 { // every open literal ran out of file
		st := s.str()
		s.report(token.Error, st.open, s.off, "unterminated string literal")
		s.strs = s.strs[:len(s.strs)-1]
	}
	s.emit(token.EOF, s.off) // the one zero-width span
	token.SortDiagnostics(s.diags)
	return s.toks, s.diags
}

type scanner struct {
	f    *token.File
	src  []byte
	mode Mode
	off  int

	toks  []token.Token
	diags []token.Diagnostic

	adjacent bool // no trivia since the previous token
	nlBefore bool // line terminator since the previous token
	quietTok bool // current token already reported

	// strs is the stack of open string literals, innermost last. A
	// literal is on it from its opening quote to its closing one, and
	// an interpolation inside it puts the scanner back on the
	// ordinary token path — where another literal may open.
	strs []strLit
}

// strLit is one open string literal.
type strLit struct {
	open      int  // offset of the opening quote, for the unterminated report
	pounds    int  // ExtendedStringLiteralDelimiter length; 0 for an ordinary literal
	multiline bool // opened with """
	interp    bool // scanning an interpolation, not the literal's own text
	depth     int  // open parens within that interpolation
}

func (s *scanner) str() *strLit {
	if len(s.strs) == 0 {
		return nil
	}
	return &s.strs[len(s.strs)-1]
}

func (s *scanner) peek(i int) byte {
	if s.off+i < len(s.src) {
		return s.src[s.off+i]
	}
	return 0
}

func (s *scanner) at(i int, c byte) bool { return s.off+i < len(s.src) && s.src[s.off+i] == c }

// ---- trivia ----

// skipShebang consumes a #! line at the very start of the file. A
// Vertex source file may be run as a script, and the line that says
// so is not program text.
func (s *scanner) skipShebang() {
	if len(s.src) >= 2 && s.src[0] == '#' && s.src[1] == '!' {
		for s.off < len(s.src) && !isNewline(s.src[s.off]) {
			s.off++
		}
	}
}

func (s *scanner) skipTrivia() {
	for s.off < len(s.src) {
		switch c := s.src[s.off]; {
		case c == ' ' || c == '\t' || c == '\v' || c == '\f' || c == 0:
			s.off++
			s.adjacent = false

		case c == '\n' || c == '\r':
			s.off++
			if c == '\r' && s.at(0, '\n') {
				s.off++
			}
			s.adjacent = false
			s.nlBefore = true

		case c == '/' && s.peek(1) == '/':
			start := s.off
			for s.off < len(s.src) && !isNewline(s.src[s.off]) {
				s.off++
			}
			s.comment(start)

		case c == '/' && s.peek(1) == '*':
			s.blockComment()

		default:
			return
		}
	}
}

// blockComment consumes a /* … */ comment. They nest: /* /* */ */ is
// one comment, which is what lets a reviewer comment out a region
// that already holds a comment.
func (s *scanner) blockComment() {
	start := s.off
	depth := 0
	for s.off < len(s.src) {
		switch {
		case s.src[s.off] == '/' && s.peek(1) == '*':
			s.off += 2
			depth++
		case s.src[s.off] == '*' && s.peek(1) == '/':
			s.off += 2
			depth--
			if depth == 0 {
				s.comment(start)
				return
			}
		default:
			s.off++
		}
	}
	s.report(token.Error, start, s.off, "unterminated '/*' comment")
	s.comment(start)
}

func (s *scanner) comment(start int) {
	for i := start; i < s.off; i++ {
		if isNewline(s.src[i]) {
			s.nlBefore = true
			break
		}
	}
	if s.mode&ScanComments != 0 {
		s.emit(token.COMMENT, start)
	}
	s.adjacent = false
}

// ---- tokens ----

func (s *scanner) scanToken() {
	s.quietTok = false
	c := s.src[s.off]

	switch {
	case c == '(' || c == ')' || c == '[' || c == ']' || c == '{' || c == '}' ||
		c == ',' || c == ':' || c == ';' || c == '@' || c == '\\':
		s.scanPunct()

	case c == '#':
		s.scanPound()

	case c == '"':
		s.openString(s.off, 0)

	case c == '`':
		s.scanEscapedIdent()

	case c == '$':
		s.scanDollarIdent()

	case isDigit(c):
		s.scanNumber()

	case isOperHead(c) || c == '.':
		s.scanOperator()

	default:
		r, w := s.rune()
		switch {
		case isIdentHead(r):
			s.scanIdent(s.off)
		case isOperHeadRune(r):
			s.scanOperator()
		// A combining mark may be carried by a name but not open
		// one. Reading the run as a name anyway is what keeps the
		// rest of the line intelligible.
		case isIdentChar(r):
			start := s.off
			s.errTok(start, start+w, "an identifier cannot begin with this character")
			s.off += w
			s.scanIdent(start)
		default:
			s.off += w
			s.errTok(s.off-w, s.off, fmt.Sprintf("invalid character %q in source file", r))
			s.emit(token.ILLEGAL, s.off-w)
		}
	}

}

// rune decodes the scalar at the cursor. An invalid encoding decodes
// as one byte of RuneError, so every path still advances.
func (s *scanner) rune() (rune, int) {
	r, w := utf8.DecodeRune(s.src[s.off:])
	if w == 0 {
		w = 1
	}
	return r, w
}

// scanPunct handles the structural punctuation — the characters that
// are not operator characters and so need no binding analysis. Braces
// and parens also drive the interpolation stack: the ')' that closes
// an interpolation returns the scanner to the literal's text.
func (s *scanner) scanPunct() {
	start := s.off
	c := s.src[s.off]
	s.off++

	var k token.Kind
	switch c {
	case '(':
		k = token.LPAREN
		if st := s.str(); st != nil && st.interp {
			st.depth++
		}
	case ')':
		k = token.RPAREN
		if st := s.str(); st != nil && st.interp {
			if st.depth == 0 {
				st.interp = false // back to the literal's own text
			} else {
				st.depth--
			}
		}
	case '[':
		k = token.LSQUARE
	case ']':
		k = token.RSQUARE
	case '{':
		k = token.LBRACE
	case '}':
		k = token.RBRACE
	case ',':
		k = token.COMMA
	case ':':
		k = token.COLON
	case ';':
		k = token.SEMI
	case '@':
		k = token.AT
	case '\\':
		k = token.BACKSLASH // a KeyPathExpression's root
	}
	s.emit(k, start)
}

// scanPound handles everything a '#' opens: a raw string or regex
// literal, one of the # words, or — for any other spelling — the bare
// POUND of a MacroExpansion, whose Identifier follows as its own
// token.
func (s *scanner) scanPound() {
	start := s.off
	n := 0
	for s.at(n, '#') {
		n++
	}
	switch {
	case s.at(n, '"'):
		s.off += n
		s.openString(start, n)
		return
	case s.at(n, '/') && s.tryRegex(start, n):
		return
	}
	if n > 1 { // ## with nothing a delimiter could belong to
		s.off += n
		s.errTok(start, s.off, "expected a string or regex literal after '#' delimiters")
		s.emit(token.ILLEGAL, start)
		return
	}

	s.off++ // the '#'
	word := s.off
	for s.off < len(s.src) {
		r, w := s.rune()
		if !isIdentChar(r) {
			break
		}
		s.off += w
	}
	if k := token.LookupPound(string(s.src[word:s.off])); k != token.POUND {
		s.emit(k, start)
		return
	}
	s.off = word // leave the identifier — MacroExpansion reads it
	s.emit(token.POUND, start)
}

// ---- operators ----

func (s *scanner) peekOperChar(i int) bool {
	if s.off+i < len(s.src) {
		b := s.src[s.off+i]
		if isOperHead(b) || b == '.' {
			return true
		}
		if b >= utf8Self {
			r, _ := utf8.DecodeRune(s.src[s.off+i:])
			return isOperChar(r)
		}
	}
	return false
}

// scanOperator consumes one maximal run of operator characters and
// decides what it is. A run that begins with '.' may contain more of
// them (`...`, `..<`); one that does not ends at the first '.', which
// is what makes `a.b` a member access and `a...b` a range.
func (s *scanner) scanOperator() {
	start := s.off
	dotted := s.src[s.off] == '.'

	if dotted && !s.peekOperChar(1) {
		// A lone '.': member access, or the head of an implicit member
		// reference. Fall through to the classification below.
		s.off++
	} else {

		if s.src[s.off] == '/' && s.tryRegex(start, 0) {
			return
		}
		for s.off < len(s.src) {
			c := s.src[s.off]
			if c == '.' && !dotted {
				break
			}
			if !isOperHead(c) && c != '.' {
				r, w := s.rune()
				if r < utf8.RuneSelf || !isOperChar(r) {
					break
				}
				s.off += w
				continue
			}
			if (c == '/' && (s.peek(1) == '/' || s.peek(1) == '*')) ||
				(c == '*' && s.peek(1) == '/' && s.off > start) {
				break // a comment starts here, and ends the operator
			}
			s.off++
		}
	}

	text := string(s.src[start:s.off])
	left := s.leftBound(start)
	right := s.rightBound(s.off, left)

	var fl token.Flags
	if left {
		fl |= token.FlagLeftBound
	}
	if right {
		fl |= token.FlagRightBound
	}

	// The reserved operators: spelled out by the grammar, so no
	// precedencegroup may claim them.
	switch text {
	case "=":
		if left != right {
			s.report(token.Warn, start, s.off,
				"'=' must have consistent whitespace on both sides")
		}
		s.emitFlags(token.ASSIGN, start, fl)
		return
	case "->":
		s.emitFlags(token.ARROW, start, fl)
		return
	case ".":
		if right && !left {
			s.emitFlags(token.PERIOD_PREFIX, start, fl)
		} else {
			s.emitFlags(token.PERIOD, start, fl)
		}
		return
	case "&":
		if right && !left { // an InOutExpression, never the binary operator
			s.emitFlags(token.AMP_PREFIX, start, fl)
			return
		}
	case "?":
		if left {
			s.emitFlags(token.QUESTION_POSTFIX, start, fl)
		} else {
			s.emitFlags(token.QUESTION_INFIX, start, fl)
		}
		return
	case "!":
		if left {
			s.emitFlags(token.EXCLAIM_POSTFIX, start, fl)
			return
		}
	case "*/":
		s.errTok(start, s.off, "unexpected end of block comment")
		s.emit(token.ILLEGAL, start)
		return
	}

	// Otherwise position is whitespace: bound on both sides or on
	// neither is infix, bound on the left alone is postfix, and bound
	// on the right alone is prefix.
	switch {
	case left == right:
		s.emitFlags(token.OPER_BINARY, start, fl)
	case left:
		s.emitFlags(token.OPER_POSTFIX, start, fl)
	default:
		s.emitFlags(token.OPER_PREFIX, start, fl)
	}
}

// leftBound reports whether the character before an operator run
// could end an operand.
func (s *scanner) leftBound(start int) bool {
	if start == 0 {
		return false
	}
	switch s.src[start-1] {
	case ' ', '\t', '\n', '\r', '\v', '\f', 0,
		'(', '[', '{', ',', ';', ':':
		return false
	case '/':
		return start < 2 || s.src[start-2] != '*' // the end of a block comment is whitespace
	}
	return true
}

// rightBound reports whether the character after an operator run
// could begin an operand.
func (s *scanner) rightBound(end int, left bool) bool {
	if end >= len(s.src) {
		return false
	}
	switch s.src[end] {
	case ' ', '\t', '\n', '\r', '\v', '\f', 0,
		')', ']', '}', ',', ';', ':':
		return false
	case '.':
		// `x^.y` reads the '^' as postfix and the '.' as a member
		// access; `^.y` reads the '^' as prefix over `.y`.
		return !left
	case '/':
		if end+1 < len(s.src) && (s.src[end+1] == '/' || s.src[end+1] == '*') {
			return false // a comment begins here, so this is whitespace
		}
	}
	return true
}

// ---- emission and diagnostics ----

func (s *scanner) emit(k token.Kind, start int) { s.emitFlags(k, start, 0) }

func (s *scanner) emitFlags(k token.Kind, start int, fl token.Flags) {
	if s.adjacent {
		fl |= token.FlagAdjacent
	}
	if s.nlBefore {
		fl |= token.FlagNLBefore
	}
	s.toks = append(s.toks, token.Token{
		Kind: k, Flags: fl,
		Pos: s.f.Pos(start), End: s.f.Pos(s.off),
	})
	s.adjacent = true
	s.nlBefore = false
}

// errTok reports at most once per token: after the first report the
// current token goes quiet.
func (s *scanner) errTok(lo, hi int, msg string) {
	if s.quietTok {
		return
	}
	s.quietTok = true
	s.report(token.Error, lo, hi, msg)
}

// report appends one diagnostic with a non-empty span clamped to the
// source.
func (s *scanner) report(sev token.Severity, lo, hi int, msg string) {
	n := len(s.src)
	if n == 0 {
		return
	}
	if hi <= lo {
		hi = lo + 1
	}
	if hi > n {
		hi = n
	}
	if lo >= hi {
		lo = hi - 1
	}
	s.diags = append(s.diags, token.Diagnostic{
		Pos: s.f.Pos(lo), End: s.f.Pos(hi), Severity: sev, Message: msg,
	})
}

// ---- character classes ----

func isDigit(c byte) bool    { return '0' <= c && c <= '9' }
func isBinDigit(c byte) bool { return c == '0' || c == '1' }
func isOctDigit(c byte) bool { return '0' <= c && c <= '7' }

func isHexDigit(c byte) bool {
	return isDigit(c) || 'a' <= c && c <= 'f' || 'A' <= c && c <= 'F'
}

// hexValue is the value of a hexadecimal digit.
func hexValue(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	default:
		return int(c-'A') + 10
	}
}

func isNewline(c byte) bool { return c == '\n' || c == '\r' }

// isOperHead is the ASCII half of OperatorHead. The Unicode scalars
// reserved for operators are in isOperChar.
func isOperHead(c byte) bool {
	switch c {
	case '/', '=', '-', '+', '!', '*', '%', '<', '>', '&', '|', '^', '~', '?':
		return true
	}
	return false
}
