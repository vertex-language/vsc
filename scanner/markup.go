package scanner

import (
	"unicode/utf8"

	"github.com/vertex-language/vsc/token"
)

// Markup is scanned only in a .vsx file, and only where the file's
// grammar leaves room for it: a `<` standing where a prefix operator
// would -- not bound on the left, bound on the right by a name or by
// `>` -- opens a tag. That is the one place markup and Vertex could
// collide, and a .vsx file gives up the prefix operator there; a .vs
// file never sees markup at all. See the RFC, proposed_vsx.md §9.1.
//
// Inside markup the scanner keeps a stack of frames, innermost last:
// the attributes of an opening tag, a closing tag, an element's
// children, and a `{…}` of code. Code is scanned as everywhere else --
// and may open markup again -- until the brace that closes it.

type frameKind uint8

const (
	frameTag      frameKind = iota // between `<name` and `>` or `/>`
	frameCloseTag                  // between `</` and `>`
	frameChildren                  // between an opening tag's `>` and its `</`
	frameCode                      // between `{` and its `}`
)

type markupFrame struct {
	kind  frameKind
	open  int // offset of what opened the frame, for the unterminated report
	depth int // for frameCode: braces open inside it
}

func (s *scanner) frame() *markupFrame {
	if len(s.frames) == 0 {
		return nil
	}
	return &s.frames[len(s.frames)-1]
}

func (s *scanner) push(k frameKind, open int) {
	s.frames = append(s.frames, markupFrame{kind: k, open: open})
}

func (s *scanner) pop() { s.frames = s.frames[:len(s.frames)-1] }

// inMarkup reports whether the cursor is in markup rather than code.
func (s *scanner) inMarkup() bool {
	f := s.frame()
	return f != nil && f.kind != frameCode
}

// opensMarkup reports whether the `<` at the cursor opens a tag.
func (s *scanner) opensMarkup() bool {
	if s.mode&ScanMarkup == 0 || s.src[s.off] != '<' {
		return false
	}
	// Not inside a string's interpolation: `"\(<b/>)"` is not markup.
	if st := s.str(); st != nil && st.interp {
		return false
	}
	if s.leftBound(s.off) {
		return false
	}
	if s.opensGenericParams() {
		return false
	}
	if s.off+1 >= len(s.src) {
		return false
	}
	if s.src[s.off+1] == '>' {
		return true
	}
	r, _ := utf8.DecodeRune(s.src[s.off+1:])
	return isIdentHead(r)
}

// openMarkup emits the `<` that opens a tag and enters its attributes.
func (s *scanner) openMarkup() {
	start := s.off
	s.off++
	s.emit(token.MARKUP_OPEN, start)
	s.push(frameTag, start)
}

// scanMarkup scans one token of markup in the innermost frame.
func (s *scanner) scanMarkup() {
	f := s.frame()
	if f.kind == frameChildren {
		s.scanChildren()
		return
	}
	s.skipTrivia()
	if s.off >= len(s.src) {
		return
	}
	s.quietTok = false
	start := s.off
	c := s.src[s.off]
	switch {
	case c == '>':
		s.off++
		s.emit(token.MARKUP_END, start)
		if f.kind == frameTag {
			f.kind = frameChildren
			return
		}
		// A closing tag ends its element: the closing tag, then the
		// element's children.
		s.pop()
		if g := s.frame(); g != nil && g.kind == frameChildren {
			s.pop()
		}
	case c == '/' && s.peek(1) == '>' && f.kind == frameTag:
		s.off += 2
		s.emit(token.MARKUP_SELF_CLOSE, start)
		s.pop()
	case c == '{' && f.kind == frameTag:
		s.off++
		s.emit(token.LBRACE, start)
		s.push(frameCode, start)
	case c == '=' && f.kind == frameTag:
		s.off++
		s.emit(token.ASSIGN, start)
	case (c == '"' || c == '\'') && f.kind == frameTag:
		s.scanMarkupString(c)
	case isMarkupNameHead(c) || c >= utf8.RuneSelf:
		r, w := s.rune()
		if c < utf8.RuneSelf || isIdentHead(r) {
			s.scanMarkupName()
			return
		}
		s.off += w
		s.errTok(start, s.off, "this character cannot appear in a tag")
		s.emit(token.ILLEGAL, start)
	default:
		s.off++
		s.errTok(start, s.off, "this character cannot appear in a tag")
		s.emit(token.ILLEGAL, start)
	}
}

// scanChildren scans what stands between an element's tags: a tag, a
// `{…}` of code, or a run of text.
func (s *scanner) scanChildren() {
	s.quietTok = false
	start := s.off
	switch s.src[s.off] {
	case '<':
		if s.peek(1) == '/' {
			s.off += 2
			s.emit(token.MARKUP_CLOSE_OPEN, start)
			s.push(frameCloseTag, start)
			return
		}
		if s.off+1 < len(s.src) {
			r, _ := utf8.DecodeRune(s.src[s.off+1:])
			if s.src[s.off+1] == '>' || isIdentHead(r) {
				s.off++
				s.emit(token.MARKUP_OPEN, start)
				s.push(frameTag, start)
				return
			}
		}
		s.off++
		s.errTok(start, s.off, `'<' cannot appear in markup text; write {"<"} or &lt;`)
		s.emit(token.MARKUP_TEXT, start)
		return
	case '{':
		s.off++
		s.emit(token.LBRACE, start)
		s.push(frameCode, start)
		return
	}
	for s.off < len(s.src) {
		c := s.src[s.off]
		if c == '<' || c == '{' {
			break
		}
		if c == '>' || c == '}' {
			s.report(token.Error, s.off, s.off+1,
				"'"+string(c)+"' cannot appear in markup text; write {\""+string(c)+"\"} or an entity")
		}
		s.off++
	}
	s.emit(token.MARKUP_TEXT, start)
}

// scanMarkupName scans a tag or attribute name: identifier characters,
// with `-` (data-p), `.` (app.Window) and `:` (class:done) inside it,
// and `--` after a `:` (style:--kit-accent).
func (s *scanner) scanMarkupName() {
	start := s.off
	for s.off < len(s.src) {
		c := s.src[s.off]
		if c == '-' || c == '.' || c == ':' {
			s.off++
			continue
		}
		r, w := s.rune()
		if !isIdentChar(r) {
			break
		}
		s.off += w
	}
	s.emit(token.MARKUP_NAME, start)
}

// scanMarkupString scans an attribute's quoted value. As in JSX there
// are no escapes: the value runs to the next matching quote.
func (s *scanner) scanMarkupString(quote byte) {
	start := s.off
	s.off++
	for s.off < len(s.src) && s.src[s.off] != quote {
		s.off++
	}
	if s.off >= len(s.src) {
		s.report(token.Error, start, s.off, "unterminated attribute value")
	} else {
		s.off++
	}
	s.emit(token.MARKUP_STRING, start)
}

func isMarkupNameHead(c byte) bool {
	return c == '_' || c == '$' || c == '-' || ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z')
}

// markupBrace keeps a `{…}` of code in step with the braces written in
// it, and reports whether the `}` at hand closes the code itself.
func (s *scanner) markupBrace(open bool) (closes bool) {
	f := s.frame()
	if f == nil || f.kind != frameCode {
		return false
	}
	if st := s.str(); st != nil && st.interp {
		return false
	}
	if open {
		f.depth++
		return false
	}
	if f.depth == 0 {
		s.pop()
		return true
	}
	f.depth--
	return false
}

// unterminatedMarkup reports markup still open at the end of the file.
func (s *scanner) unterminatedMarkup() {
	for len(s.frames) > 0 {
		f := s.frame()
		switch f.kind {
		case frameCode:
			s.report(token.Error, f.open, s.off, "unterminated '{' in markup")
		case frameChildren:
			s.report(token.Error, f.open, s.off, "element has no closing tag")
		default:
			s.report(token.Error, f.open, s.off, "unterminated tag")
		}
		s.pop()
	}
}

// opensGenericParams reports whether a `<` at the cursor follows a name
// being declared, and so opens its generic parameters: `func |> <T, U>`
// has to be spelled with the space, or `|><` would be one operator, and
// `func f <T>()` may be. Markup never stands there -- no expression can
// follow a declaration's name -- so one token of look-behind settles it.
func (s *scanner) opensGenericParams() bool {
	n := len(s.toks)
	if n == 0 {
		return false
	}
	last := s.toks[n-1].Kind
	if last == token.INIT || last == token.SUBSCRIPT {
		return true
	}
	if n < 2 || (last != token.IDENT && !last.IsOperator()) {
		return false
	}
	before := s.toks[n-2]
	switch before.Kind {
	case token.FUNC, token.STRUCT, token.CLASS, token.ENUM, token.PROTOCOL, token.TYPEALIAS:
		return true
	case token.IDENT:
		// The contextual introducers: `actor A <T>`, `macro m <T>`.
		word := string(s.src[s.f.Offset(before.Pos):s.f.Offset(before.End)])
		return word == "actor" || word == "macro"
	}
	return false
}
