// Package iface writes a module's interface: the public face of a compiled
// module as Vertex source with the bodies taken out, which is what another
// module is compiled against.
//
// Types are written the way Vertex spells them -- int32 rather than Int32 --
// since the file is Vertex. The two are the same type; see types.VertexName.
package iface

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/vertex-language/vsc/analyzer"
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/token"
	"github.com/vertex-language/vsc/types"
)

// FormatVersion is the interface format version written to module headers.
const FormatVersion = "1.0"

// Extension is what an interface file is called.
const Extension = ".vinterface"

// A Module is what Print needs to know about what it is describing.
type Module struct {
	Name  string
	Files []*ast.File
	Units []*token.File
	Info  *analyzer.Info
}

// Print writes m's public interface.
func Print(w io.Writer, m Module) error {
	p := &printer{w: w, m: m}
	p.line("// vertex-interface-format-version: %s", FormatVersion)
	p.line("// vertex-module-name: %s", m.Name)
	p.line("")
	// What the module imports, once each, before anything that may name
	// a type from one of them: a module reading this interface reads
	// these too, the way it reads a swiftinterface's.
	seen := map[string]bool{}
	for i, f := range m.Files {
		if i < len(m.Units) {
			p.file = m.Units[i]
		}
		for _, stmt := range f.Stmts {
			decl, ok := stmt.(*ast.DeclStmt)
			if !ok {
				continue
			}
			imp, ok := decl.D.(*ast.ImportDecl)
			if !ok || len(imp.Path) == 0 || p.file == nil {
				continue
			}
			name := imp.Path[0].Text(p.file)
			if name == "" || seen[name] {
				continue
			}
			seen[name] = true
			p.line("import %s", name)
		}
	}
	if len(seen) > 0 {
		p.line("")
	}
	for i, f := range m.Files {
		if i < len(m.Units) {
			p.file = m.Units[i]
		}
		for _, stmt := range f.Stmts {
			decl, ok := stmt.(*ast.DeclStmt)
			if !ok {
				continue
			}
			p.decl(decl.D)
		}
	}
	return p.err
}

type printer struct {
	w    io.Writer
	m    Module
	file *token.File
	err  error
	// isolated is whether the type whose members are being written is
	// @MainActor, which its members then are unless they say otherwise.
	isolated bool
}

func (p *printer) line(format string, args ...any) {
	if p.err != nil {
		return
	}
	_, p.err = fmt.Fprintf(p.w, format+"\n", args...)
}

// decl writes one top-level declaration, or nothing where it is not
// part of the module's public face.
func (p *printer) decl(d ast.Decl) {
	switch n := d.(type) {
	case *ast.FuncDecl:
		// A method declared with a receiver belongs to its type, and the
		// nominal it is on writes it. Writing it here too would put it at
		// module scope, where it is not.
		if n.Recv != nil || !p.exported(n.Mods) {
			return
		}
		sym, _ := p.m.Info.Defs[n.Name].(*analyzer.FuncSymbol)
		if sym == nil {
			return
		}
		acc := access(p.text(n.Mods))
		if sym.Signature().Isolated {
			acc = "@MainActor " + acc
		}
		p.line("%s", p.function(acc, sym.Name(), sym.Signature()))
		p.line("")

	case *ast.StructDecl:
		if !p.exported(n.Mods) {
			return
		}
		p.nominal("struct", n.Name, n.Mods)

	case *ast.ClassDecl:
		if !p.exported(n.Mods) {
			return
		}
		p.nominal("class", n.Name, n.Mods)

	case *ast.EnumDecl:
		if !p.exported(n.Mods) {
			return
		}
		p.enum(n)
	}
}

// nominal writes a struct or class declaration with stored properties and methods in declaration order.
func (p *printer) nominal(keyword string, name *ast.Ident, mods []*ast.Modifier) {
	sym, _ := p.m.Info.Defs[name].(*analyzer.TypeNameSymbol)
	if sym == nil {
		return
	}
	var fields, computed, statics []*types.Field
	var methods []*types.Method
	var inits []*types.Signature
	var subscripts []*types.Subscript
	var inherits []string
	switch t := sym.Type().Underlying().(type) {
	case *types.Struct:
		fields, methods, inits = t.Fields, t.Methods, t.Inits
		computed, statics, subscripts = t.Computed, t.Statics, t.Subscripts
		inherits = protocolNames(t.Conformances)
	case *types.Class:
		fields, methods, inits = t.Fields, t.Methods, t.Inits
		computed, statics, subscripts = t.Computed, t.Statics, t.Subscripts
		if t.Superclass != nil {
			inherits = append(inherits, typeText(t.Superclass))
		}
		inherits = append(inherits, protocolNames(t.Conformances)...)
	default:
		return
	}
	super := inheritText(inherits)

	acc := access(p.text(mods))
	// A @MainActor type says so, and its members that are not
	// (`nonisolated`) say so each; see isolation.
	isolated := p.m.Info.MainActor[sym.Type().Underlying()]
	if isolated {
		acc = "@MainActor " + acc
	}
	prevIsolated := p.isolated
	p.isolated = isolated
	defer func() { p.isolated = prevIsolated }()
	p.line("%s %s %s%s {", acc, keyword, p.ident(name), super)
	for _, f := range fields {
		p.property(f, false)
	}
	// A computed property has no storage, so what a client needs is the
	// accessors it may call, not a layout.
	for _, f := range computed {
		p.property(f, false)
	}
	// A static property is the type's own storage, reached through its
	// addressor rather than through any instance.
	for _, f := range statics {
		p.property(f, true)
	}
	// Emit declared initializers: the public ones, which are all another
	// module may call.
	for _, sig := range inits {
		if sig == nil || !sig.Exported {
			continue
		}
		p.line("  %s%s", p.isolation(sig.Isolated), p.initializer(sig))
	}
	p.methods(methods)
	p.subscripts(subscripts)
	p.line("}")
	p.line("")
}

// subscripts writes the ones a client may use, with the accessors it
// may call. A parameter is written with its label, or with `_` where it
// has none -- which is what a subscript's parameter has by default, the
// other way round from a function's.
func (p *printer) subscripts(subs []*types.Subscript) {
	for _, sub := range subs {
		if sub == nil || !sub.Exported {
			continue
		}
		var b strings.Builder
		b.WriteString("  public ")
		if sub.IsStatic {
			b.WriteString("static ")
		}
		b.WriteString("subscript(")
		for i, param := range sub.Params {
			if i > 0 {
				b.WriteString(", ")
			}
			q := *param
			if q.Label == "" {
				q.Label = "_"
			}
			b.WriteString(p.paramText(&q))
		}
		b.WriteString(") -> ")
		b.WriteString(typeText(sub.Result))
		if sub.Settable {
			b.WriteString(" { get set }")
		} else {
			b.WriteString(" { get }")
		}
		p.line("%s", b.String())
	}
}

// methods writes the ones a client may call. A method that is not public
// is this module's own business, and writing it put internal helpers in
// the interface for anyone to call.
func (p *printer) methods(methods []*types.Method) {
	for _, m := range methods {
		if m == nil || m.Sig == nil || !m.Exported {
			continue
		}
		acc := "public"
		if m.IsStatic {
			acc = "public static"
		} else if m.IsMutating {
			acc = "public mutating"
		}
		p.line("  %s", p.function(p.isolation(m.Sig.Isolated)+acc, m.Name, m.Sig))
	}
}

// property writes one stored, computed or static property of a type.
//
// A stored property is written as it was declared, because a client lays
// the type out itself and so needs its storage. A computed one is written
// with the accessors a client may call, which is what `{ get }` says and
// all a client can use.
func (p *printer) property(f *types.Field, static bool) {
	if f == nil || f.Type == nil {
		return
	}
	kw := "var"
	if f.IsConst {
		kw = "let"
	}
	access := "public"
	if !f.Exported {
		// A stored property that is not public is still written, as
		// internal: a client lays the type out, and needs every one.
		// Anything else that is not public is not the client's.
		if f.IsComputed || static {
			return
		}
		access = "internal"
	}
	prefix := "  " + p.isolation(f.Isolated) + access + " "
	if static {
		prefix = "  " + p.isolation(f.Isolated) + access + " static "
	}
	if !f.IsComputed {
		p.line("%s%s %s: %s", prefix, kw, escape(f.Name), typeText(f.Type))
		return
	}
	// A computed property is always `var`: it has accessors, not storage.
	accessors := "{ get }"
	if f.HasSetter {
		accessors = "{ get set }"
	}
	p.line("%svar %s: %s %s", prefix, escape(f.Name), typeText(f.Type), accessors)
}

// isolation is what a member of a type writes before its access where
// its isolation differs from the type's: @MainActor on one the type
// does not make so, nonisolated on one that opts out.
func (p *printer) isolation(isolated bool) string {
	switch {
	case isolated && !p.isolated:
		return "@MainActor "
	case !isolated && p.isolated:
		return "nonisolated "
	}
	return ""
}

// enum writes enum cases and methods in declaration order.
func (p *printer) enum(n *ast.EnumDecl) {
	sym, _ := p.m.Info.Defs[n.Name].(*analyzer.TypeNameSymbol)
	if sym == nil {
		return
	}
	e, ok := sym.Type().Underlying().(*types.Enum)
	if !ok {
		return
	}
	acc := access(p.text(n.Mods))
	isolated := p.m.Info.MainActor[e]
	if isolated {
		acc = "@MainActor " + acc
	}
	prevIsolated := p.isolated
	p.isolated = isolated
	defer func() { p.isolated = prevIsolated }()
	p.line("%s enum %s%s {", acc, p.ident(n.Name), inheritText(protocolNames(e.Conformances)))
	for _, c := range e.Cases {
		if c == nil {
			continue
		}
		if c.AssociatedType != nil {
			p.line("  case %s(%s)", escape(c.Name), typeText(c.AssociatedType))
			continue
		}
		p.line("  case %s", escape(c.Name))
	}
	for _, f := range e.Computed {
		p.property(f, false)
	}
	for _, f := range e.Statics {
		p.property(f, true)
	}
	p.methods(e.Methods)
	p.line("}")
	p.line("")
}

// function formats a function declaration signature without a body.
func (p *printer) function(acc, name string, sig *types.Signature) string {
	var b strings.Builder
	b.WriteString(acc)
	// An operator of one operand is written before it; see analyzer.
	if isOperator(name) && len(sig.Params) == 1 {
		b.WriteString(" prefix")
	}
	b.WriteString(" func ")
	b.WriteString(escape(name))
	b.WriteString("(")
	for i, param := range sig.Params {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(p.paramText(param))
	}
	b.WriteString(")")
	if sig.Async {
		b.WriteString(" async")
	}
	if sig.Throws {
		b.WriteString(" throws")
	}
	if sig.Results != nil && !isVoid(sig.Results) {
		b.WriteString(" -> ")
		b.WriteString(typeText(sig.Results))
	}
	return b.String()
}

// initializer writes an init's declaration.
func (p *printer) initializer(sig *types.Signature) string {
	var b strings.Builder
	b.WriteString("public init(")
	for i, param := range sig.Params {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(p.paramText(param))
	}
	b.WriteString(")")
	if sig.Async {
		b.WriteString(" async")
	}
	if sig.Throws {
		b.WriteString(" throws")
	}
	return b.String()
}

// paramText formats a single parameter declaration with labels and type.
func (p *printer) paramText(param *types.Param) string {
	var b strings.Builder
	switch {
	case param.Label == "" || param.Label == param.Name:
		b.WriteString(escape(param.Name))
	default:
		b.WriteString(escape(param.Label))
		b.WriteString(" ")
		b.WriteString(escape(param.Name))
	}
	b.WriteString(": ")
	// Ownership belongs to the type, not before the label: a parameter is
	// written `into buffer: inout [UInt8]`, which is what reads back.
	if param.Ownership != types.DefaultOwnership {
		b.WriteString(param.Ownership.String())
		b.WriteString(" ")
	}
	if param.Type != nil {
		b.WriteString(typeText(param.Type))
	}
	if param.Variadic {
		b.WriteString("...")
	}
	// A default is part of the call, not of the callee: a client evaluates
	// it at the call site, so the interface carries the expression as it
	// was written.
	if text := p.defaultText(param); text != "" {
		b.WriteString(" = ")
		b.WriteString(text)
	}
	return b.String()
}

// defaultText is a parameter's default value as it was written, or "" for
// a parameter with none, or one whose source this cannot recover.
func (p *printer) defaultText(param *types.Param) string {
	if !param.HasDefault || p.m.Info == nil {
		return ""
	}
	expr := p.m.Info.Defaults[param]
	if expr == nil {
		return ""
	}
	file := p.m.Info.DefaultFiles[expr]
	if file == nil {
		return ""
	}
	src := file.Slice(expr.Pos(), expr.End())
	if len(src) == 0 {
		return ""
	}
	// An interface declaration is one line, so a default written over
	// several becomes one.
	return strings.Join(strings.Fields(string(src)), " ")
}

// typeText writes a type the way Vertex spells it.
//
// A printed type is a type expression -- "[UInt8]", "(code: Int32, message:
// String)", "ArraySlice<UInt8>" -- so the universe's names appear in it as
// whole identifiers, and each is rewritten to Vertex's. The scan is over
// identifiers rather than a search for substrings, so a type called
// MyInt32 keeps its name.
func typeText(t types.Type) string {
	if t == nil {
		return ""
	}
	return vertexTypes(t.String())
}

// vertexTypes rewrites the universe names in a printed type expression.
func vertexTypes(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		if !identStart(s[i]) {
			b.WriteByte(s[i])
			i++
			continue
		}
		j := i + 1
		for j < len(s) && identPart(s[j]) {
			j++
		}
		word := s[i:j]
		if name, ok := types.VertexName(word); ok {
			b.WriteString(name)
		} else {
			b.WriteString(word)
		}
		i = j
	}
	return b.String()
}

func identStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func identPart(c byte) bool { return identStart(c) || (c >= '0' && c <= '9') }

// escape spells a name so it reads back as an identifier. A declaration
// may be named with a keyword -- `default` is a common one -- and the
// backticks that made it an identifier are not part of the name, so an
// interface has to put them back.
func escape(name string) string {
	// `_` is how a parameter says it has no label. It is spelled as the
	// keyword it is, not as an identifier.
	if name == "" || name == "_" || token.Lookup(name) == token.IDENT {
		return name
	}
	return "`" + name + "`"
}

func isVoid(t types.Type) bool {
	b, ok := t.Underlying().(*types.Basic)
	return ok && b.Kind() == types.Void
}

// exported reports whether modifiers include `public` or `open`.
func (p *printer) exported(mods []*ast.Modifier) bool {
	for _, m := range p.text(mods) {
		if m == "public" || m == "open" {
			return true
		}
	}
	return false
}

func access(mods []string) string {
	for _, m := range mods {
		if m == "open" {
			return "open"
		}
	}
	return "public"
}

func (p *printer) text(mods []*ast.Modifier) []string {
	out := make([]string, 0, len(mods))
	for _, m := range mods {
		if m == nil || m.Name == nil || p.file == nil {
			continue
		}
		out = append(out, m.Name.Text(p.file))
	}
	sort.Strings(out)
	return out
}

func (p *printer) ident(id *ast.Ident) string {
	if id == nil || p.file == nil {
		return "?"
	}
	return escape(id.Text(p.file))
}

// protocolNames is the names of the protocols a type conforms to, as its
// declaration wrote them.
func protocolNames(ps []*types.Protocol) []string {
	var out []string
	for _, pr := range ps {
		if pr != nil && pr.Name != "" {
			out = append(out, pr.Name)
		}
	}
	return out
}

// inheritText is an inheritance clause, or nothing for an empty list.
func inheritText(names []string) string {
	if len(names) == 0 {
		return ""
	}
	return ": " + strings.Join(names, ", ")
}

// isOperator reports whether a function's name is an operator's.
func isOperator(name string) bool {
	if name == "" {
		return false
	}
	for i := 0; i < len(name); i++ {
		if !strings.ContainsRune("&@/=><*!|+?%-~^.", rune(name[i])) {
			return false
		}
	}
	return true
}
