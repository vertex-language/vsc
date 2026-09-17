package analyzer

import (
	"strings"

	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/scanner"
	"github.com/vertex-language/vsc/token"
)

// ValueKind indicates the kind of decoded constant held by a Value.
type ValueKind uint8

const (
	NoValue ValueKind = iota
	IntValue
	FloatValue
	StringValue
	BoolValue
)

// Value represents a decoded literal constant.
type Value struct {
	Kind  ValueKind
	Int   uint64
	Float float64
	Str   string
	Bool  bool
}

// IsValid reports whether v holds a value.
func (v Value) IsValid() bool { return v.Kind != NoValue }

// valueOf decodes a literal expression and records it in Info.Values.
func (c *checker) valueOf(expr ast.Expr) Value {
	var v Value
	switch e := expr.(type) {
	case *ast.BasicLit:
		v = c.basicValue(e)
	case *ast.StringLit:
		v = c.stringValue(e)
	default:
		return Value{}
	}
	if v.IsValid() {
		c.info.Values[expr] = v
	}
	return v
}

func (c *checker) basicValue(e *ast.BasicLit) Value {
	text := string(c.file.Slice(e.Lo, e.Hi))
	switch e.Kind {
	case token.INT_LIT:
		n, ok := scanner.DecodeInt(text)
		if !ok {
			c.errorf(e.Pos(), "integer literal '%s' overflows when stored into 'Int'", text)
			return Value{}
		}
		return Value{Kind: IntValue, Int: n}

	case token.FLOAT_LIT:
		f, ok := scanner.DecodeFloat(text)
		if !ok {
			c.errorf(e.Pos(), "floating-point literal '%s' overflows when stored into 'Double'", text)
			return Value{}
		}
		return Value{Kind: FloatValue, Float: f}

	case token.TRUE:
		return Value{Kind: BoolValue, Bool: true}
	case token.FALSE:
		return Value{Kind: BoolValue, Bool: false}
	}
	return Value{}
}

// stringValue decodes a string literal, including multiline indentation stripping.
func (c *checker) stringValue(e *ast.StringLit) Value {
	indent := ""
	if e.Multiline {
		p := c.file.Position(e.Close)
		indent = scanner.Indent(string(c.file.LineText(p.Line)))
	}

	var whole strings.Builder
	interpolated := false
	for i, seg := range e.Segments {
		text, ok := seg.(*ast.StringText)
		if !ok {
			interpolated = true
			continue
		}
		raw := string(c.file.Slice(text.Lo, text.Hi))

		// Multiline indentation stripping.
		if e.Multiline {
			if i == 0 {
				raw = strings.TrimPrefix(raw, "\r")
				raw = strings.TrimPrefix(raw, "\n")
			}
			if i == len(e.Segments)-1 {
				raw = strings.TrimSuffix(raw, indent)
				raw = strings.TrimSuffix(raw, "\n")
				raw = strings.TrimSuffix(raw, "\r")
			}
			var ok bool
			raw, ok = stripBody(raw, indent, i == 0)
			if !ok {
				c.errorf(text.Pos(), "insufficient indentation: a line must be indented at least as far as the closing delimiter")
			}
		}

		decoded, ok := scanner.DecodeText(raw, e.Pounds)
		if !ok {
			c.errorf(text.Pos(), "invalid escape sequence in literal")
		}
		c.info.Values[text] = Value{Kind: StringValue, Str: decoded}
		whole.WriteString(decoded)
	}
	if interpolated {
		return Value{}
	}
	return Value{Kind: StringValue, Str: whole.String()}
}

// stripBody removes common indent from each line of a multiline string segment.
func stripBody(raw, indent string, first bool) (string, bool) {
	if indent == "" {
		return raw, true
	}
	lines := strings.Split(raw, "\n")
	ok := true
	for i, line := range lines {
		if i == 0 && !first {
			continue
		}
		cr := strings.HasSuffix(line, "\r")
		body := strings.TrimSuffix(line, "\r")
		switch {
		case strings.HasPrefix(body, indent):
			body = body[len(indent):]
		case strings.TrimLeft(body, " \t") == "":
			body = "" // a blank line need not be indented
		default:
			ok = false
		}
		if cr {
			body += "\r"
		}
		lines[i] = body
	}
	return strings.Join(lines, "\n"), ok
}
