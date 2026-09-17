package analyzer

import (
	"strings"
	"testing"
)

// TestCheckLetMutation holds the checker to swiftc's words for writing
// through a `let`: a mutating operator on the name or on a collection's
// subscript, an assignment through a subscript, and a mutating method.
//
// Every case below was run past swiftc first; the messages are its.
func TestCheckLetMutation(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string // the message expected, or "" for a clean check
	}{
		{"operator on a let", "func f() { let x = 1; x += 1 }\n",
			"left side of mutating operator isn't mutable: 'x' is a 'let' constant"},
		{"operator on a let array's element", "func f() { let a = [1]; a[0] += 1 }\n",
			"left side of mutating operator isn't mutable: 'a' is a 'let' constant"},
		{"operator on a let dictionary's default", "func f() { let d = [\"a\": 1]; d[\"a\", default: 0] += 1 }\n",
			"left side of mutating operator isn't mutable: 'd' is a 'let' constant"},
		{"assignment through a let dictionary's default", "func f() { let d = [\"a\": 1]; d[\"b\", default: 0] = 2 }\n",
			"cannot assign through subscript: 'd' is a 'let' constant"},
		{"mutating method on a let array", "func f() { let a = [1]; a.append(2) }\n",
			"cannot use mutating member on immutable value: 'a' is a 'let' constant"},
		{"all of it on vars", "func f() { var x = 1; x += 1; var a = [1]; a[0] += 1; a.append(2)\n" +
			"var d = [\"a\": 1]; d[\"a\", default: 0] += 1; d[\"b\", default: 0] = 2 }\n", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			msgs := checkSnippet(t, c.src)
			if c.want == "" {
				if len(msgs) > 0 {
					t.Errorf("want a clean check, got:\n  %s", strings.Join(msgs, "\n  "))
				}
				return
			}
			if len(msgs) != 1 || msgs[0] != c.want {
				t.Errorf("want %q, got:\n  %s", c.want, strings.Join(msgs, "\n  "))
			}
		})
	}
}
