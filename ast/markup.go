package ast

import (
	"strings"
	"unicode"

	"github.com/vertex-language/vsc/token"
)

// Markup is the JSX a .vsx file may write where an expression begins.
// Every `{…}` in it is a MarkupCode: its contents are parsed as a
// closure's, so `{count}`, `{e in go(e)}` and a handler of several lines
// are one shape, and what the braces mean -- a value, or the closure
// itself where a function is wanted -- is the checker's to say.

// MarkupElement is `<Name attrs…>children…</Name>`, `<Name attrs… />`, or
// a fragment, `<>children…</>`, whose Name is nil.
type MarkupElement struct {
	Span
	Open        token.Pos    // the `<`
	Name        *MarkupName  // nil for a fragment
	Attrs       []MarkupAttr // *MarkupAttribute and *MarkupSpread, in written order
	SelfClosing bool         // `/>`
	Children    []MarkupChild
	CloseName   *MarkupName // the closing tag's name; nil where there is none
	Close       token.Pos   // the last `>`
}

// MarkupName is a tag or attribute name as written: `div`, `app.Window`,
// `class:done`, `data-p`, `style:--kit-accent`.
type MarkupName struct {
	Span
}

// Text is the name's spelling.
func (n *MarkupName) Text(f *token.File) string {
	if n == nil {
		return ""
	}
	return f.String(n.Lo, n.Hi)
}

// MarkupAttr is one of an opening tag's attributes.
type MarkupAttr interface {
	Node
	markupAttr()
}

// MarkupAttribute is `name`, `name="text"` or `name={code}`. Value is nil
// for a bare name, and a *MarkupString or *MarkupCode otherwise.
type MarkupAttribute struct {
	Span
	Name   *MarkupName
	Assign token.Pos
	Value  Node
}

// MarkupSpread is `{...attrs}` among the attributes.
type MarkupSpread struct {
	Span
	Lbrace token.Pos
	X      Expr
	Rbrace token.Pos
}

// MarkupString is a quoted attribute value, quotes included. As in JSX
// it has no escapes.
type MarkupString struct {
	Span
}

// Value is the string's text without its quotes.
func (s *MarkupString) Value(f *token.File) string {
	t := f.String(s.Lo, s.Hi)
	if len(t) >= 2 {
		return t[1 : len(t)-1]
	}
	return ""
}

// MarkupChild is what an element holds: *MarkupText, *MarkupCode or
// *MarkupElement.
type MarkupChild interface {
	Node
	markupChild()
}

// MarkupText is text between tags, as written; JSX's whitespace rules
// apply to it when it is read (Value).
type MarkupText struct {
	Span
}

// Value is the text as markup means it: whitespace folded as JSX folds
// it, and character references decoded. Text of whitespace alone that
// spans lines is "".
func (t *MarkupText) Value(f *token.File) string {
	return decodeEntities(foldText(f.String(t.Lo, t.Hi)))
}

// MarkupCode is `{…}` in markup. Body holds what the braces hold, parsed
// as a closure; an empty body is `{}` or `{/* a comment */}`.
type MarkupCode struct {
	Span
	Body *ClosureExpr
}

func (*MarkupElement) exprNode() {}

func (*MarkupAttribute) markupAttr() {}
func (*MarkupSpread) markupAttr()    {}

func (*MarkupText) markupChild()    {}
func (*MarkupCode) markupChild()    {}
func (*MarkupElement) markupChild() {}

// foldText applies JSX's whitespace rule: lines are trimmed where they
// meet a line break, lines left empty are dropped, and the rest are
// joined by one space. Text on one line keeps its spaces.
func foldText(s string) string {
	if !strings.ContainsAny(s, "\n\r") {
		return s
	}
	lines := strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	var out []string
	for i, l := range lines {
		if i > 0 {
			l = strings.TrimLeft(l, " \t")
		}
		if i < len(lines)-1 {
			l = strings.TrimRight(l, " \t")
		}
		if l != "" {
			out = append(out, l)
		}
	}
	return strings.Join(out, " ")
}

// decodeEntities decodes the character references markup text may use
// for what it may not hold raw: &lt; &gt; &amp; &quot; &apos; &#123;
// &#x7B; and &nbsp;.
func decodeEntities(s string) string {
	if !strings.Contains(s, "&") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '&' {
			b.WriteByte(s[i])
			continue
		}
		end := strings.IndexByte(s[i:], ';')
		if end < 0 || end > 10 {
			b.WriteByte('&')
			continue
		}
		name := s[i+1 : i+end]
		var r rune = -1
		switch name {
		case "lt":
			r = '<'
		case "gt":
			r = '>'
		case "amp":
			r = '&'
		case "quot":
			r = '"'
		case "apos":
			r = '\''
		case "nbsp":
			r = '\u00a0'
		default:
			if strings.HasPrefix(name, "#x") || strings.HasPrefix(name, "#X") {
				r = parseRune(name[2:], 16)
			} else if strings.HasPrefix(name, "#") {
				r = parseRune(name[1:], 10)
			}
		}
		if r < 0 {
			b.WriteByte('&')
			continue
		}
		b.WriteRune(r)
		i += end
	}
	return b.String()
}

func parseRune(digits string, base int) rune {
	if digits == "" {
		return -1
	}
	var n rune
	for _, d := range digits {
		var v rune
		switch {
		case '0' <= d && d <= '9':
			v = d - '0'
		case base == 16 && 'a' <= d && d <= 'f':
			v = d - 'a' + 10
		case base == 16 && 'A' <= d && d <= 'F':
			v = d - 'A' + 10
		default:
			return -1
		}
		n = n*rune(base) + v
		if n > unicode.MaxRune {
			return -1
		}
	}
	return n
}
