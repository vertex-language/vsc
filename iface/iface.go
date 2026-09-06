// Package iface writes a module's public interface.
//
// The format is Swift's own answer, and so is the reason for it: a
// .swiftinterface is valid Swift with the bodies taken out. Anything
// that can read the language can read a module's API, which is why
// this compiler needs no second parser and no binary module format to
// compile one module against another -- the interface is a source
// file, and the front end already reads source files.
//
// # What the interface has to carry, and why
//
// Layout. swiftc without -enable-library-evolution compiles a client
// against another module's types exactly as if they were local: a
// field read is a struct_extract at a fixed offset, and a struct
// crosses a call in registers. The client can only do that if it
// knows the fields, in order, so the interface lists them -- and a
// reordering of a public struct's stored properties is then a
// breaking change, which is precisely what Swift says it is outside
// of library evolution.
//
// The same goes for a class: the order of its methods is the order of
// its vtable slots, and a subclass in another module fills the slots
// its base declared. And for an enum: a case's position is its tag.
//
// Resilience -- the mode where none of that is baked in and a field
// is read through a getter call instead -- is what Swift turns on
// with -enable-library-evolution and @frozen turns back off per type.
// It is not implemented here. What is implemented is the default, and
// the limitation it carries is the one Swift's default carries.
//
// # Where this is still more permissive than Swift
//
// A public struct's memberwise initializer is internal in Swift: a
// client of the module cannot call Point(x:y:) unless the type
// declares a public init of its own. This compiler lets it, because
// an initializer a type declares itself is not lowered yet -- so
// enforcing the rule today would leave a public struct with no way to
// be made at all. The rule goes in when `public init` does, and until
// then a program that builds here may not build under swiftc.
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

// FormatVersion is written at the top of every interface, so that a
// reader can refuse one it does not understand rather than
// misreading it.
const FormatVersion = "1.0"

// Extension is what an interface file is called.
const Extension = ".vertexinterface"

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
		if !p.exported(n.Mods) {
			return
		}
		sym, _ := p.m.Info.Defs[n.Name].(*analyzer.FuncSymbol)
		if sym == nil {
			return
		}
		p.line("%s", p.function(access(p.text(n.Mods)), sym.Name(), sym.Signature()))
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

// nominal writes a struct or a class: its stored properties in
// declaration order, then its methods in declaration order. Both
// orders are load-bearing -- the first is the layout and the second
// is the vtable.
func (p *printer) nominal(keyword string, name *ast.Ident, mods []*ast.Modifier) {
	sym, _ := p.m.Info.Defs[name].(*analyzer.TypeNameSymbol)
	if sym == nil {
		return
	}
	var fields []*types.Field
	var methods []*types.Method
	var super string
	switch t := sym.Type().Underlying().(type) {
	case *types.Struct:
		fields, methods = t.Fields, t.Methods
	case *types.Class:
		fields, methods = t.Fields, t.Methods
		if t.Superclass != nil {
			super = ": " + t.Superclass.String()
		}
	default:
		return
	}

	acc := access(p.text(mods))
	p.line("%s %s %s%s {", acc, keyword, p.ident(name), super)
	for _, f := range fields {
		if f == nil || f.Type == nil {
			continue
		}
		kw := "var"
		if f.IsConst {
			kw = "let"
		}
		p.line("  public %s %s: %s", kw, f.Name, f.Type)
	}
	for _, m := range methods {
		if m == nil || m.Sig == nil {
			continue
		}
		p.line("  %s", p.function("public", m.Name, m.Sig))
	}
	p.line("}")
	p.line("")
}

// enum writes an enum's cases in declaration order, which is what
// gives each its tag.
func (p *printer) enum(n *ast.EnumDecl) {
	sym, _ := p.m.Info.Defs[n.Name].(*analyzer.TypeNameSymbol)
	if sym == nil {
		return
	}
	e, ok := sym.Type().Underlying().(*types.Enum)
	if !ok {
		return
	}
	p.line("%s enum %s {", access(p.text(n.Mods)), p.ident(n.Name))
	for _, c := range e.Cases {
		if c == nil {
			continue
		}
		if c.AssociatedType != nil {
			p.line("  case %s(%s)", c.Name, c.AssociatedType)
			continue
		}
		p.line("  case %s", c.Name)
	}
	for _, m := range e.Methods {
		if m == nil || m.Sig == nil {
			continue
		}
		p.line("  %s", p.function("public", m.Name, m.Sig))
	}
	p.line("}")
	p.line("")
}

// function is a declaration with no body: the labels and the names
// both, because a call names one and the callee names the other, and
// dropping either would change what the declaration means.
func (p *printer) function(acc, name string, sig *types.Signature) string {
	var b strings.Builder
	b.WriteString(acc)
	b.WriteString(" func ")
	b.WriteString(name)
	b.WriteString("(")
	for i, param := range sig.Params {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(paramText(param))
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
		b.WriteString(sig.Results.String())
	}
	return b.String()
}

// paramText writes one parameter with both of its names.
//
// A parameter written with one name has the same label and name, and
// is printed once; `_ n: Int` has no label and keeps the underscore,
// because that is what says a call passes it without one.
func paramText(param *types.Param) string {
	var b strings.Builder
	if param.Ownership != types.DefaultOwnership {
		b.WriteString(param.Ownership.String())
		b.WriteString(" ")
	}
	switch {
	case param.Label == "" || param.Label == param.Name:
		b.WriteString(param.Name)
	default:
		b.WriteString(param.Label)
		b.WriteString(" ")
		b.WriteString(param.Name)
	}
	b.WriteString(": ")
	if param.Type != nil {
		b.WriteString(param.Type.String())
	}
	if param.Variadic {
		b.WriteString("...")
	}
	return b.String()
}

func isVoid(t types.Type) bool {
	b, ok := t.Underlying().(*types.Basic)
	return ok && b.Kind() == types.Void
}

// exported reports whether a declaration is part of the module's
// public face. Only public and open cross a module boundary; internal
// is the module itself, which is the whole point of the boundary.
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
	return id.Text(p.file)
}
