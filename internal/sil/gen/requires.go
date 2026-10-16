package gen

import (
	"strings"

	"github.com/vertex-language/ir"
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/internal/sil"
	"github.com/vertex-language/vsc/token"
)

// attrRequires is `@requires(aes, clmul)`: the capabilities -- the IR's,
// spelled as it spells them -- whose instructions the function's body may
// use whatever the target's baseline is. It is how a library's hardware
// path is compiled; the caller picks it with CPU.hasAES and friends.
const attrRequires = "requires"

// requires records a function's @requires on its SIL function, refusing
// a capability the IR does not have.
func (g *gen) requires(d *ast.FuncDecl, f *sil.Func) {
	for _, a := range d.Attrs {
		if a == nil || attrNameOf(g, a) != attrRequires {
			continue
		}
		var names []string
		for _, t := range a.Tokens {
			switch t.Kind {
			case token.IDENT:
				name := string(g.file.Slice(t.Pos, t.End))
				if _, ok := ir.ParseCapability(name); !ok {
					g.errorAt(d, "'@requires' names '"+name+"', which is not a capability; "+
						"they are aes, clmul, sha1, sha256, sha512, sha3, sm3, sm4, crc32, crc32c")
					return
				}
				names = append(names, name)
			case token.COMMA:
			default:
				g.errorAt(d, "'@requires' takes capabilities separated by commas, as in '@requires(aes, clmul)'")
				return
			}
		}
		if len(names) == 0 {
			g.errorAt(d, "'@requires' needs the capabilities it requires, as in '@requires(aes, clmul)'")
			return
		}
		f.SetAttr(sil.AttrRequires + strings.Join(names, ","))
	}
}
