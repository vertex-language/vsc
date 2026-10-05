package doc

import "strings"

// A block is one piece of a doc comment, as go/doc/comment reads one:
// a paragraph, a heading, a list, or code -- the lines indented past the
// text around them.
type block struct {
	kind  blockKind
	lines []string
}

type blockKind int

const (
	paragraph blockKind = iota
	heading
	list
	code
)

// blocks splits a doc comment into its blocks.
func blocks(text string) []block {
	var out []block
	lines := strings.Split(text, "\n")
	for i := 0; i < len(lines); {
		l := lines[i]
		switch {
		case strings.TrimSpace(l) == "":
			i++
		case indented(l):
			// Code runs to the first line that is not indented, blank
			// lines inside it kept.
			j := i
			for j < len(lines) && (indented(lines[j]) || strings.TrimSpace(lines[j]) == "") {
				j++
			}
			for j > i && strings.TrimSpace(lines[j-1]) == "" {
				j--
			}
			out = append(out, block{code, dedentCode(lines[i:j])})
			i = j
		case strings.HasPrefix(l, "```"):
			j := i + 1
			for j < len(lines) && !strings.HasPrefix(lines[j], "```") {
				j++
			}
			out = append(out, block{code, lines[i+1 : min(j, len(lines))]})
			i = j + 1
		case strings.HasPrefix(l, "# "):
			out = append(out, block{heading, []string{strings.TrimSpace(l[2:])}})
			i++
		case listItem(l) != "":
			var items []string
			for i < len(lines) && strings.TrimSpace(lines[i]) != "" && !indented(lines[i]) && !strings.HasPrefix(lines[i], "# ") {
				if it := listItem(lines[i]); it != "" {
					items = append(items, it)
				} else if len(items) > 0 {
					items[len(items)-1] += " " + strings.TrimSpace(lines[i])
				}
				i++
			}
			out = append(out, block{list, items})
		default:
			var para []string
			for i < len(lines) && strings.TrimSpace(lines[i]) != "" && !indented(lines[i]) &&
				!strings.HasPrefix(lines[i], "```") && !strings.HasPrefix(lines[i], "# ") &&
				(len(para) == 0 || listItem(lines[i]) == "") {
				para = append(para, strings.TrimSpace(lines[i]))
				i++
			}
			out = append(out, block{paragraph, para})
		}
	}
	return out
}

func indented(l string) bool {
	return strings.HasPrefix(l, "\t") || strings.HasPrefix(l, "    ")
}

// listItem is a bulleted or numbered line's text, or "".
func listItem(l string) string {
	t := strings.TrimLeft(l, " ")
	for _, b := range []string{"- ", "* ", "+ "} {
		if strings.HasPrefix(t, b) {
			return strings.TrimSpace(t[len(b):])
		}
	}
	n := 0
	for n < len(t) && t[n] >= '0' && t[n] <= '9' {
		n++
	}
	if n > 0 && n+1 < len(t) && (t[n] == '.' || t[n] == ')') && t[n+1] == ' ' {
		return strings.TrimSpace(t[n+2:])
	}
	return ""
}

// dedentCode takes off the indentation the code's lines share.
func dedentCode(lines []string) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = strings.ReplaceAll(l, "\t", "    ")
	}
	common := -1
	for _, l := range out {
		if strings.TrimSpace(l) == "" {
			continue
		}
		n := len(l) - len(strings.TrimLeft(l, " "))
		if common < 0 || n < common {
			common = n
		}
	}
	for i, l := range out {
		if len(l) >= common && common > 0 {
			out[i] = l[common:]
		} else {
			out[i] = strings.TrimLeft(l, " ")
		}
	}
	return out
}

// Synopsis is the first sentence of a doc comment, as go/doc's is: for
// an index entry.
func Synopsis(text string) string {
	bs := blocks(text)
	if len(bs) == 0 || bs[0].kind != paragraph {
		return ""
	}
	s := strings.Join(bs[0].lines, " ")
	for i := 0; i+1 < len(s); i++ {
		if (s[i] == '.' || s[i] == '!' || s[i] == '?') && s[i+1] == ' ' {
			// Not the dot of an abbreviation like "e.g."
			if i > 0 && s[i-1] >= 'A' && s[i-1] <= 'Z' && (i < 2 || s[i-2] == ' ') {
				continue
			}
			return s[:i+1]
		}
	}
	return s
}
