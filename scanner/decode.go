package scanner

import (
	"strconv"
	"strings"
)

// The decoders. The scanner reads a literal's extent and checks that
// it is well formed; these turn the text it delimited into the value
// it denotes, which is the last lexical question about a literal and
// the first thing any consumer of one needs.
//
// They are separate from scanning because the two happen at different
// times. A literal is scanned once, and decoded only where its value
// is wanted — a constant expression, a case label, an argument to a
// macro. Everything here is a function of the text alone.

// DecodeInt returns the value of an IntegerLiteral's spelling: a
// decimal run, or one prefixed 0b, 0o or 0x, with '_' anywhere but
// the front. ok is false where the digits do not fit in 64 bits,
// which is what makes an overflowing literal reportable rather than
// silently wrong.
//
// The sign is not here. `-1` is a prefix operator over the literal,
// which is what the tree says, so this returns the magnitude.
func DecodeInt(text string) (v uint64, ok bool) {
	s := strings.ReplaceAll(text, "_", "")
	base := 10
	if len(s) > 2 && s[0] == '0' {
		switch s[1] {
		case 'b':
			base, s = 2, s[2:]
		case 'o':
			base, s = 8, s[2:]
		case 'x':
			base, s = 16, s[2:]
		}
	}
	if s == "" {
		return 0, false
	}
	v, err := strconv.ParseUint(s, base, 64)
	return v, err == nil
}

// DecodeFloat returns the value of a FloatingPointLiteral: a decimal
// with an optional fraction and exponent, or a hexadecimal one whose
// p exponent is a power of two. ok is false where the value is not
// finite, which is how a literal too large to represent is reported.
func DecodeFloat(text string) (float64, bool) {
	s := strings.ReplaceAll(text, "_", "")
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// DecodeText returns the value of one run of a string literal's own
// text: the escape sequences resolved, everything else as written.
// pounds is the length of the literal's delimiter, which is what an
// escape must carry to be one — inside `#"…"#`, `\n` is two
// characters and `\#n` is a newline.
//
// ok is false where an escape names nothing. The scanner has already
// reported that, so a caller decoding a literal it scanned may ignore
// the flag; one decoding text from elsewhere may not.
func DecodeText(text string, pounds int) (string, bool) {
	esc := "\\" + strings.Repeat("#", pounds)
	if !strings.Contains(text, esc) {
		return text, true
	}
	var b strings.Builder
	b.Grow(len(text))
	ok := true
	for i := 0; i < len(text); {
		if !strings.HasPrefix(text[i:], esc) {
			b.WriteByte(text[i])
			i++
			continue
		}
		i += len(esc)
		if i >= len(text) {
			return b.String(), false
		}
		switch c := text[i]; c {
		case '0':
			b.WriteByte(0)
			i++
		case '\\':
			b.WriteByte('\\')
			i++
		case 't':
			b.WriteByte('\t')
			i++
		case 'n':
			b.WriteByte('\n')
			i++
		case 'r':
			b.WriteByte('\r')
			i++
		case '"':
			b.WriteByte('"')
			i++
		case '\'':
			b.WriteByte('\'')
			i++
		case 'u':
			r, n, good := decodeScalar(text[i:])
			if !good {
				ok = false
			} else {
				b.WriteRune(r)
			}
			i += n

		// An escaped newline is a line continuation: the break is
		// not part of the value, and neither is the indentation that
		// follows it, which the multiline strip has already removed.
		case '\n':
			i++
		case '\r':
			i++
			if i < len(text) && text[i] == '\n' {
				i++
			}
		default:
			ok = false
			b.WriteByte(c)
			i++
		}
	}
	return b.String(), ok
}

// decodeScalar reads `u{ hex }` at the front of s and returns the
// scalar, how many bytes it spanned, and whether it named one.
func decodeScalar(s string) (r rune, n int, ok bool) {
	if len(s) < 3 || s[0] != 'u' || s[1] != '{' {
		return 0, 1, false
	}
	end := strings.IndexByte(s, '}')
	if end < 0 {
		return 0, len(s), false
	}
	digits := s[2:end]
	if digits == "" || len(digits) > 8 {
		return 0, end + 1, false
	}
	v, err := strconv.ParseUint(digits, 16, 32)
	if err != nil || v > 0x10FFFF || (v >= 0xD800 && v <= 0xDFFF) {
		return 0, end + 1, false
	}
	return rune(v), end + 1, true
}

// StripIndent removes a multiline literal's indentation from text.
//
// The closing delimiter's own indentation is what is stripped, from
// every line of the literal — that is how a `"""` literal is written
// inside an indented declaration without the indentation becoming
// part of the string. A line indented less than the delimiter is a
// mistake, and ok says so.
//
// text is the literal's whole body, from the line break after the
// opening delimiter to the one before the closing delimiter, both
// already removed by the caller.
func StripIndent(text, indent string) (string, bool) {
	if indent == "" {
		return text, true
	}
	lines := strings.Split(text, "\n")
	ok := true
	for i, line := range lines {
		trimmed := strings.TrimSuffix(line, "\r")
		cr := len(trimmed) != len(line)
		switch {
		case strings.HasPrefix(trimmed, indent):
			trimmed = trimmed[len(indent):]
		case strings.TrimLeft(trimmed, " \t") == "":
			trimmed = "" // a blank line need not be indented
		default:
			ok = false
		}
		if cr {
			trimmed += "\r"
		}
		lines[i] = trimmed
	}
	return strings.Join(lines, "\n"), ok
}

// Indent returns the whitespace a multiline literal's closing
// delimiter is written after — the prefix StripIndent removes. line
// is the text of the line the delimiter closes on.
func Indent(line string) string {
	return line[:len(line)-len(strings.TrimLeft(line, " \t"))]
}
