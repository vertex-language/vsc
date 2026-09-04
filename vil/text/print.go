// Package text prints a VIL module in Swift's SIL syntax.
//
// Exactly Swift's, because the point of printing it at all is that
// the output can be diffed against `swiftc -emit-silgen`. A form that
// is nearly SIL would give a diff that has to be read rather than
// run, which is the whole value gone.
//
// What is not cloned is symbol mangling: Swift's encodes Swift's
// module names and declaration grammar, and ours is not that. The
// differential harness normalizes both sides — symbols to positions,
// and value numbers with them — before comparing.
package text

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/vertex-language/vsc/vil"
)

// Print writes m in SIL syntax.
func Print(w io.Writer, m *vil.Module) error {
	p := &printer{w: w}
	p.module(m)
	return p.err
}

// String is Print into a string.
func String(m *vil.Module) string {
	var b strings.Builder
	Print(&b, m)
	return b.String()
}

// Func prints one function, which is what a test usually wants.
func Func(w io.Writer, f *vil.Func) error {
	p := &printer{w: w}
	p.fn(f)
	return p.err
}

type printer struct {
	w   io.Writer
	err error

	// names holds the %n each value prints as. SIL numbers values
	// densely per function in the order they are defined, which is
	// not the order they were created, so the numbering is computed
	// per function rather than carried on the value.
	names map[*vil.Value]int
}

func (p *printer) printf(format string, args ...any) {
	if p.err != nil {
		return
	}
	_, p.err = fmt.Fprintf(p.w, format, args...)
}

func (p *printer) module(m *vil.Module) {
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

func (p *printer) fn(f *vil.Func) {
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

// number assigns the %n each value prints as: densely, in definition
// order, block arguments before the instructions of their block.
func (p *printer) number(f *vil.Func) {
	p.names = map[*vil.Value]int{}
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

func (p *printer) block(b *vil.Block) {
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

// commentColumn is where SIL's printer puts a trailing comment. It
// pads to it, and so does this, because a diff whose lines differ
// only in whitespace is a diff that has to be read rather than run.
const commentColumn = 50

// comment appends a trailing comment at the column SIL uses, with a
// single space where the line is already past it.
func comment(line, text string) string {
	if n := len([]rune(line)); n < commentColumn {
		line += strings.Repeat(" ", commentColumn-n)
	} else {
		line += " "
	}
	return line + "// " + text
}

// def prints a definition with its type and ownership, which is how a
// block argument is written.
func (p *printer) def(v *vil.Value) string {
	own := v.Ownership().String()
	if own != "" {
		return fmt.Sprintf("%s : %s %s", p.ref(v), own, v.Type())
	}
	return fmt.Sprintf("%s : %s", p.ref(v), v.Type())
}

// ref prints a use: just the number.
func (p *printer) ref(v *vil.Value) string {
	if v == nil {
		return "undef"
	}
	if n, ok := p.names[v]; ok {
		return "%" + strconv.Itoa(n)
	}
	return "%" + strconv.Itoa(v.ID())
}

func (p *printer) inst(in *vil.Inst) {
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

// operands writes what follows the opcode. Each instruction's text
// form is Swift's, which is why this is a switch and not a loop.
func (p *printer) operands(in *vil.Inst) {
	aux := in.Aux()
	args := in.Args()

	switch in.Op() {
	case vil.FunctionRef:
		p.printf(" @%s : $%s", aux.Name, funcTypeOf(in))

	case vil.IntegerLiteral:
		p.printf(" %s, %d", aux.Type, aux.Int)

	case vil.StringLiteral:
		p.printf(" %s %q", attrList(aux.Attrs), aux.Text)

	case vil.Metatype:
		p.printf(" %s", aux.Type)

	case vil.AllocStack, vil.AllocRef, vil.AllocBox:
		p.printf("%s %s", attrPrefix(aux.Attrs), aux.Type)

	case vil.Load, vil.Store, vil.CopyAddr, vil.BeginAccess,
		vil.BeginBorrow, vil.MoveValue, vil.MarkUninitialized:
		p.printf("%s %s", attrPrefix(aux.Attrs), p.refs(args))

	case vil.StructExtract, vil.StructElementAddr, vil.RefElementAddr,
		vil.UncheckedEnumData, vil.ClassMethod:
		p.printf(" %s, #%s", p.refs(args), aux.Member)

	case vil.TupleExtract:
		p.printf(" %s, %d", p.refs(args), aux.Int)

	case vil.ProjectBox:
		p.printf(" %s, %d", p.refs(args), aux.Int)

	case vil.Struct, vil.Tuple:
		p.printf(" %s (%s)", aux.Type, p.refs(args))

	case vil.Enum:
		p.printf(" %s, #%s", aux.Type, aux.Member)
		if len(args) > 0 {
			p.printf(", %s", p.refs(args))
		}

	case vil.WitnessMethod:
		p.printf(" #%s", aux.Member)

	case vil.Apply, vil.PartialApply:
		p.printf("%s %s(%s)", attrPrefix(aux.Attrs), p.ref(args[0]), p.refs(args[1:]))

	// `debug_value %0, let, name "b", argno 1` — the binding
	// qualifier comes before the name and everything else after,
	// which is the order SIL writes them in.
	case vil.DebugValue:
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

	case vil.Br:
		p.printf(" %s", aux.Dest.Label())
		if len(aux.Args) > 0 {
			p.printf("(%s)", p.refs(aux.Args))
		}

	case vil.CondBr:
		p.printf(" %s, %s", p.ref(args[0]), aux.Dest.Label())
		if len(aux.Args) > 0 {
			p.printf("(%s)", p.refs(aux.Args))
		}
		p.printf(", %s", aux.Else.Label())
		if len(aux.ElseArgs) > 0 {
			p.printf("(%s)", p.refs(aux.ElseArgs))
		}

	case vil.SwitchEnum:
		p.printf(" %s", p.ref(args[0]))
		for _, c := range aux.Cases {
			if c.Member == "" {
				p.printf(", default %s", c.Dest.Label())
				continue
			}
			p.printf(", case #%s: %s", c.Member, c.Dest.Label())
		}

	case vil.Unreachable, vil.Unwind:
		// no operands

	default:
		if len(args) > 0 {
			p.printf(" %s", p.refs(args))
		}
	}
}

// funcTypeOf is the type a function_ref names, printed after the
// symbol as SIL writes it.
func funcTypeOf(in *vil.Inst) string {
	if r := in.Result(); r != nil {
		return strings.TrimPrefix(r.Type().String(), "$")
	}
	return "<unknown>"
}

func (p *printer) refs(vs []*vil.Value) string {
	parts := make([]string, len(vs))
	for i, v := range vs {
		parts[i] = p.ref(v)
	}
	return strings.Join(parts, ", ")
}

// linkage prints a linkage with its trailing space, or nothing for
// public, which SIL writes by saying nothing.
func linkage(l vil.Linkage) string {
	if l == vil.Public {
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

// attrPrefix prints an instruction's bracketed modifiers, each
// preceded by a space: ` [copy]`.
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

func (p *printer) vtable(t *vil.VTable) {
	p.printf("sil_vtable %s {\n", t.Class)
	for _, e := range t.Entries {
		p.printf("  #%s: @%s\n", e.Member, e.Impl)
	}
	p.printf("}\n\n")
}

func (p *printer) witness(t *vil.WitnessTable) {
	p.printf("sil_witness_table %s%s: %s module %s {\n",
		linkage(t.Linkage), t.Type, t.Protocol, t.Module)
	for _, e := range t.Entries {
		p.printf("  method #%s: @%s\n", e.Member, e.Impl)
	}
	p.printf("}\n\n")
}
