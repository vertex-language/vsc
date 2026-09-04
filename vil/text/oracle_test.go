package text

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/vertex-language/vsc/types"
	"github.com/vertex-language/vsc/vil"
)

// The differential harness.
//
// testdata holds SIL as `swiftc -emit-silgen` printed it. A test
// builds the same function in VIL, prints it, normalizes both sides,
// and requires them to agree. Once vil/gen exists this is what it
// will be held to; until then it is what holds the text form to
// Swift's, which is the only reason the text form is Swift's.
//
// Normalization is the one licence taken, and it covers exactly three
// things that differ without meaning anything:
//
//   - symbols, because VIL does not clone Swift's mangling
//   - the '%n' numbering, which follows from the symbols
//   - the trailing '// user:' and '// id:' cross-references, which
//     restate the def-use graph the instructions already carry

// silAttrs are the `@` words that are part of the language rather
// than names of things. They must survive normalization untouched:
// `@owned` against `@guaranteed` is exactly the kind of difference
// this harness exists to catch.
var silAttrs = map[string]bool{
	"@convention": true, "@owned": true, "@guaranteed": true,
	"@unowned": true, "@in": true, "@in_guaranteed": true,
	"@inout": true, "@inout_aliasable": true, "@out": true,
	"@error": true, "@yields": true, "@yield_once": true,
	"@yield_many": true, "@thin": true, "@thick": true,
	"@objc_metatype": true, "@escaping": true, "@noescape": true,
	"@callee_guaranteed": true, "@callee_owned": true,
	"@autoreleased": true, "@opened": true, "@async": true,
	"@objc": true, "@block_storage": true, "@sil_isolated": true,
	"@sil_sending": true, "@pack_guaranteed": true, "@pack_owned": true,
	"@dynamic_self": true, "@moveOnly": true, "@isolated": true,
}

var (
	reSymbol  = regexp.MustCompile(`@\$?[A-Za-z_$][A-Za-z0-9_$]*`)
	reValue   = regexp.MustCompile(`%[0-9]+`)
	reComment = regexp.MustCompile(`\s*//.*$`)
	reOpened  = regexp.MustCompile(`@opened\("[^"]*"`)
)

// normalize reduces SIL text to what two compilers must agree on.
func normalize(text string) string {
	var out []string
	symbols := map[string]string{}
	values := map[string]string{}

	for _, line := range strings.Split(text, "\n") {
		line = reComment.ReplaceAllString(line, "")
		if strings.TrimSpace(line) == "" {
			continue
		}
		line = reOpened.ReplaceAllString(line, `@opened("_"`)
		line = reSymbol.ReplaceAllStringFunc(line, func(s string) string {
			if silAttrs[s] {
				return s
			}
			if _, ok := symbols[s]; !ok {
				symbols[s] = "@f" + itoa(len(symbols))
			}
			return symbols[s]
		})
		line = reValue.ReplaceAllStringFunc(line, func(s string) string {
			if _, ok := values[s]; !ok {
				values[s] = "%" + itoa(len(values))
			}
			return values[s]
		})
		out = append(out, strings.TrimRight(line, " "))
	}
	return strings.Join(out, "\n")
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// TestMatchesSwiftSIL builds `func borrows(_ b: Box) -> Int` in VIL
// and requires it to print as swiftc printed the same function.
func TestMatchesSwiftSIL(t *testing.T) {
	want, err := os.ReadFile("testdata/borrows.sil")
	if err != nil {
		t.Skip("no testdata")
	}

	box := types.NewNamed("Box", "", &types.Class{Name: "Box"})
	intT := vil.Object(types.Typ[types.Int])

	m := vil.NewModule("a", vil.StageRaw)
	f := m.Func("borrows").SetLinkage(vil.Hidden).SetAttr("ossa")
	b := f.Param(vil.Object(box), vil.ParamGuaranteed)
	f.SetResult(intT, vil.ResultUnowned)

	bb := f.Entry()
	bb.DebugValue(b, "b", "let", "argno 1")
	addr := bb.RefElementAddr(b, "Box.n", intT)
	access := bb.BeginAccess(addr, "read", "dynamic")
	v := bb.Load(access, "trivial")
	bb.EndAccess(access)
	bb.Return(v)

	got := normalize(funcText(t, f))
	if w := normalize(string(want)); got != w {
		t.Errorf("VIL and SIL disagree.\n--- vil\n%s\n--- sil\n%s", got, w)
	}
}

// TestOwnedParameter is `func consumes(_ b: __owned Box) -> Box`: the
// function that made every ownership decision this IR exists for —
// borrow, copy, end the borrow, destroy what was given, return what
// was made.
func TestOwnedParameter(t *testing.T) {
	want, err := os.ReadFile("testdata/consumes.sil")
	if err != nil {
		t.Skip("no testdata")
	}

	box := types.NewNamed("Box", "", &types.Class{Name: "Box"})
	m := vil.NewModule("a", vil.StageRaw)
	f := m.Func("consumes").SetLinkage(vil.Hidden).SetAttr("ossa")
	b := f.Param(vil.Object(box), vil.ParamOwned)
	f.SetResult(vil.Object(box), vil.ResultOwned)

	bb := f.Entry()
	bb.DebugValue(b, "b", "let", "argno 1")
	borrow := bb.BeginBorrow(b)
	copied := bb.CopyValue(borrow)
	bb.EndBorrow(borrow)
	bb.DestroyValue(b)
	bb.Return(copied)

	got := normalize(funcText(t, f))
	if w := normalize(string(want)); got != w {
		t.Errorf("VIL and SIL disagree.\n--- vil\n%s\n--- sil\n%s", got, w)
	}

	// And the ownership the builder gave each value is the ownership
	// the text says: the copy is owned, the borrow is not.
	if copied.Ownership() != vil.Owned {
		t.Errorf("copy_value produced %v, want @owned", copied.Ownership())
	}
	if borrow.Ownership() != vil.Guaranteed {
		t.Errorf("begin_borrow produced %v, want @guaranteed", borrow.Ownership())
	}
	if got := len(b.Consumers()); got != 1 {
		t.Errorf("the owned parameter has %d consumers, want exactly one", got)
	}
}
