package text

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/vertex-language/vsc/internal/sil"
)

// Print writes m in SIL syntax.
func Print(w io.Writer, m *sil.Module) error {
	p := &printer{w: w}
	p.module(m)
	return p.err
}

// String is Print into a string.
func String(m *sil.Module) string {
	var b strings.Builder
	Print(&b, m)
	return b.String()
}

// Func prints one function, which is what a test usually wants.
func Func(w io.Writer, f *sil.Func) error {
	p := &printer{w: w}
	p.fn(f)
	return p.err
}

type printer struct {
	w   io.Writer
	err error

	// names maps values to dense function-local %n indices in definition order.
	names map[*sil.Value]int
}

func (p *printer) printf(format string, args ...any) {
	if p.err != nil {
		return
	}
	_, p.err = fmt.Fprintf(p.w, format, args...)
}

func (p *printer) module(m *sil.Module) {
	p.printf("sil_stage %s\n\n", m.Stage())
	for _, im := range m.Imports() {
		p.printf("import %s\n", im)
	}
	if len(m.Imports()) > 0 {
		p.printf("\n")
	}
	for _, g := range m.Globals() {
		p.printf("sil_global %s@%s : %s\n\n", linkage(g.Linkage()), g.Name(), g.Type())
	}
	for _, f := range m.Funcs() {
		p.fn(f)
		p.printf("\n")
	}
	for _, t := range m.VTables() {
		p.vtable(t)
	}
	for _, t := range m.WitnessTables() {
		p.witness(t)
	}
}

func (p *printer) fn(f *sil.Func) {
	p.printf("sil %s%s@%s : $%s", linkage(f.Linkage()), attrs(f.Attrs()), f.Name(), f.Type())
	if f.IsDeclaration() {
		p.printf("\n")
		return
	}
	p.printf(" {\n")

	p.number(f)
	for i, b := range f.Blocks() {
		if i > 0 {
			p.printf("\n")
		}
		p.block(b)
	}
	p.printf("} // end sil function '%s'\n", f.Name())
}

// number assigns dense %n IDs in definition order per function.
func (p *printer) number(f *sil.Func) {
	p.names = map[*sil.Value]int{}
	n := 0
	for _, b := range f.Blocks() {
		for _, a := range b.Args() {
			p.names[a] = n
			n++
		}
		for _, in := range b.Insts() {
			for _, r := range in.Results() {
				p.names[r] = n
				n++
			}
		}
	}
}

func (p *printer) block(b *sil.Block) {
	head := b.Label()
	if args := b.Args(); len(args) > 0 {
		parts := make([]string, len(args))
		for i, a := range args {
			parts[i] = p.def(a)
		}
		head += "(" + strings.Join(parts, ", ") + ")"
	}
	head += ":"

	if preds := b.Preds(); len(preds) > 0 && !b.IsEntry() {
		labels := make([]string, len(preds))
		for i, q := range preds {
			labels[i] = q.Label()
		}
		sort.Strings(labels)
		head = comment(head, "Preds: "+strings.Join(labels, " "))
	}
	p.printf("%s\n", head)
	for _, in := range b.Insts() {
		p.inst(in)
	}
}

// commentColumn is the column at which trailing comments are padded.
const commentColumn = 50

// comment appends a trailing comment padded to commentColumn.
func comment(line, text string) string {
	if n := len([]rune(line)); n < commentColumn {
		line += strings.Repeat(" ", commentColumn-n)
	} else {
		line += " "
	}
	return line + "// " + text
}

// def formats a block argument definition with its type and ownership.
func (p *printer) def(v *sil.Value) string {
	own := v.Ownership().String()
	if own != "" {
		return fmt.Sprintf("%s : %s %s", p.ref(v), own, v.Type())
	}
	return fmt.Sprintf("%s : %s", p.ref(v), v.Type())
}

// ref formats an SSA value reference (%n).
func (p *printer) ref(v *sil.Value) string {
	if v == nil {
		return "undef"
	}
	if n, ok := p.names[v]; ok {
		return "%" + strconv.Itoa(n)
	}
	return "%" + strconv.Itoa(v.ID())
}

func (p *printer) inst(in *sil.Inst) {
	p.printf("  ")
	if rs := in.Results(); len(rs) == 1 {
		p.printf("%s = ", p.ref(rs[0]))
	} else if len(rs) > 1 {
		parts := make([]string, len(rs))
		for i, r := range rs {
			parts[i] = p.ref(r)
		}
		p.printf("(%s) = ", strings.Join(parts, ", "))
	}
	p.printf("%s", in.Op())
	p.operands(in)
	p.printf("\n")
}

// operands prints the text representation of instruction operands.
func (p *printer) operands(in *sil.Inst) {
	aux := in.Aux()
	args := in.Args()

	switch in.Op() {
	case sil.FunctionRef:
		p.printf(" @%s : $%s", aux.Name, funcTypeOf(in))

	case sil.IntegerLiteral:
		p.printf(" %s, %d", aux.Type, aux.Int)

	case sil.FloatLiteral:
		p.printf(" %s, 0x%016X", aux.Type, uint64(aux.Int))

	case sil.StringLiteral:
		p.printf(" %s %q", attrList(aux.Attrs), aux.Text)

	case sil.Metatype:
		p.printf(" %s", aux.Type)

	case sil.GlobalAddr:
		p.printf(" @%s : %s", aux.Name, aux.Type.Address())

	case sil.AllocStack, sil.AllocRef:
		// Named binding declaration vs temporary stack slot.
		if aux.Name != "" {
			p.printf(" %s", aux.Type)
			for _, a := range aux.Attrs {
				p.printf(", %s", a)
			}
			p.printf(", name %q", aux.Name)
			break
		}
		p.printf("%s %s", attrPrefix(aux.Attrs), aux.Type)

	case sil.Load, sil.BeginAccess, sil.BeginBorrow, sil.MoveValue,
		sil.MarkUninitialized:
		p.printf("%s %s", attrPrefix(aux.Attrs), p.refs(args))

	// store %1 to [attr] %2
	case sil.Store, sil.CopyAddr:
		p.printf(" %s to%s %s", p.ref(args[0]), attrPrefix(aux.Attrs), p.ref(args[1]))

	case sil.Assign:
		p.printf(" %s to %s", p.ref(args[0]), p.ref(args[1]))

	// alloc_box ${ var Int }, var, name "total"
	case sil.AllocBox:
		p.printf(" %s", aux.Type)
		for _, a := range aux.Attrs {
			p.printf(", %s", a)
		}
		if aux.Name != "" {
			p.printf(", name %q", aux.Name)
		}

	case sil.StructExtract, sil.StructElementAddr, sil.RefElementAddr,
		sil.UncheckedEnumData, sil.UncheckedTakeEnumDataAddr, sil.ClassMethod:
		p.printf(" %s, #%s", p.refs(args), aux.Member)

	case sil.TupleExtract:
		p.printf(" %s, %d", p.refs(args), aux.Int)

	case sil.ProjectBox:
		p.printf(" %s, %d", p.refs(args), aux.Int)

	case sil.Struct:
		p.printf(" %s (%s)", aux.Type, p.refs(args))

	// tuple (%0, %1)
	case sil.Tuple:
		p.printf(" (%s)", p.refs(args))

	case sil.Enum:
		p.printf(" %s, #%s", aux.Type, aux.Member)
		if len(args) > 0 {
			p.printf(", %s", p.refs(args))
		}

	case sil.BuiltinCall:
		p.printf(" %q(%s)", aux.Name, p.refs(args))
		if r := in.Result(); r != nil {
			p.printf(" : %s", r.Type())
		}

	case sil.CondFail:
		p.printf(" %s, %q", p.refs(args), aux.Text)

	case sil.WitnessMethod:
		p.printf(" #%s", aux.Member)

	case sil.Apply, sil.PartialApply:
		p.printf("%s %s(%s)", attrPrefix(aux.Attrs), p.ref(args[0]), p.refs(args[1:]))
		if len(args) > 0 && args[0] != nil {
			p.printf(" : %s", args[0].Type())
		}

	// debug_value %0, let, name "b", ...
	case sil.DebugValue:
		p.printf(" %s", p.refs(args))
		for _, a := range aux.Attrs {
			if a == "let" || a == "var" {
				p.printf(", %s", a)
			}
		}
		p.printf(", name %q", aux.Name)
		for _, a := range aux.Attrs {
			if a != "let" && a != "var" {
				p.printf(", %s", a)
			}
		}

	case sil.Br:
		p.printf(" %s", aux.Dest.Label())
		if len(aux.Args) > 0 {
			p.printf("(%s)", p.refs(aux.Args))
		}

	case sil.CondBr:
		p.printf(" %s, %s", p.ref(args[0]), aux.Dest.Label())
		if len(aux.Args) > 0 {
			p.printf("(%s)", p.refs(aux.Args))
		}
		p.printf(", %s", aux.Else.Label())
		if len(aux.ElseArgs) > 0 {
			p.printf("(%s)", p.refs(aux.ElseArgs))
		}

	case sil.SwitchEnum:
		p.printf(" %s", p.ref(args[0]))
		for _, c := range aux.Cases {
			if c.Member == "" {
				p.printf(", default %s", c.Dest.Label())
				continue
			}
			p.printf(", case #%s: %s", c.Member, c.Dest.Label())
		}

	case sil.Unreachable, sil.Unwind:
		// no operands

	default:
		if len(args) > 0 {
			p.printf(" %s", p.refs(args))
		}
	}
}

// funcTypeOf returns the function type string without the leading '$'.
func funcTypeOf(in *sil.Inst) string {
	if r := in.Result(); r != nil {
		return strings.TrimPrefix(r.Type().String(), "$")
	}
	return "<unknown>"
}

func (p *printer) refs(vs []*sil.Value) string {
	parts := make([]string, len(vs))
	for i, v := range vs {
		parts[i] = p.ref(v)
	}
	return strings.Join(parts, ", ")
}

// linkage returns the linkage string with trailing space, or "" for Public.
func linkage(l sil.Linkage) string {
	if l == sil.Public {
		return ""
	}
	return string(l) + " "
}

// attrs prints a function's bracketed attributes.
func attrs(list []string) string {
	var b strings.Builder
	for _, a := range list {
		b.WriteString("[" + a + "] ")
	}
	return b.String()
}

// attrPrefix formats bracketed instruction attributes (e.g. " [copy]").
func attrPrefix(list []string) string {
	var b strings.Builder
	for _, a := range list {
		b.WriteString(" [" + a + "]")
	}
	return b.String()
}

func attrList(list []string) string {
	return strings.Join(list, " ")
}

func (p *printer) vtable(t *sil.VTable) {
	p.printf("sil_vtable %s {\n", t.Class)
	for _, e := range t.Entries {
		p.printf("  #%s: @%s\n", e.Member, e.Impl)
	}
	p.printf("}\n\n")
}

func (p *printer) witness(t *sil.WitnessTable) {
	p.printf("sil_witness_table %s%s: %s module %s {\n",
		linkage(t.Linkage), t.Type, t.Protocol, t.Module)
	for _, e := range t.Entries {
		p.printf("  method #%s: @%s\n", e.Member, e.Impl)
	}
	p.printf("}\n\n")
}
