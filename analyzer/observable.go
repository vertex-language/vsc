package analyzer

import (
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/token"
)

// observableAttr is the attribute's spelling: `@Observable final class
// Store { … }` (proposed_vsx.md §5.3).
const observableAttr = "Observable"

// lowerObservables rewrites the @Observable classes among decls, and
// among the types nested in them.
func (c *checker) lowerObservables(decls []ast.Decl) {
	for _, d := range decls {
		var body *ast.MemberBlock
		switch d := d.(type) {
		case *ast.ClassDecl:
			c.lowerObservable(d)
			body = d.Body
		case *ast.StructDecl:
			body = d.Body
		case *ast.EnumDecl:
			body = d.Body
		case *ast.ExtensionDecl:
			body = d.Body
		}
		if body == nil {
			continue
		}
		var nested []ast.Decl
		for _, m := range body.Members {
			if x, ok := m.(ast.Decl); ok {
				nested = append(nested, x)
			}
		}
		c.lowerObservables(nested)
	}
}

// lowerObservable rewrites an @Observable class's stored `var`s, in place,
// before its members are read: each
//
//	var Todos: [Todo] = []
//
// becomes its signal, which `$Todos` names, and a property read and written
// through it --
//
//	var $Todos = state.Signal<[Todo]>([])
//	var Todos: [Todo] { get { return $Todos.Value } set { $Todos.Value = newValue } }
//
// -- so reading Todos inside an effect or a live part follows it, and
// writing it tells what read it. A var set only in init starts as
// `state.Signal<T>()`, holding nothing until then. `let`s, static, lazy
// and computed properties are left as they are. The attribute goes, so a
// class read twice is rewritten once.
func (c *checker) lowerObservable(d *ast.ClassDecl) {
	at := -1
	for i, a := range d.Attrs {
		if a == nil {
			continue
		}
		switch n := a.Name.(type) {
		case *ast.IdentType:
			if n.Name != nil && n.Name.Text(c.file) == observableAttr {
				at = i
			}
		case *ast.MemberType:
			if n.Name != nil && n.Name.Text(c.file) == observableAttr {
				at = i
			}
		}
	}
	if at < 0 || d.Body == nil {
		return
	}
	d.Attrs = append(d.Attrs[:at:at], d.Attrs[at+1:]...)
	var members []ast.Node
	for _, mem := range d.Body.Members {
		v, ok := mem.(*ast.VarDecl)
		if !ok || v.Kind != token.VAR || isStatic(v.Mods) || c.hasModifier(v.Mods, "lazy") ||
			c.hasModifier(v.Mods, "weak") || c.hasModifier(v.Mods, "unowned") || len(v.Bindings) != 1 {
			members = append(members, mem)
			continue
		}
		b := v.Bindings[0]
		if b.Body != nil || b.Accessors != nil || len(v.Attrs) > 0 {
			members = append(members, mem)
			continue
		}
		storage, prop := c.observed(v, b)
		if storage == nil {
			members = append(members, mem)
			continue
		}
		members = append(members, storage, prop)
	}
	d.Body.Members = members
}

// observed is a stored var's signal and the property over it, or nils
// where the var cannot be observed (reported).
func (c *checker) observed(v *ast.VarDecl, b *ast.PatternBinding) (*ast.VarDecl, *ast.VarDecl) {
	var typ ast.Type
	pat := b.Pat
	if tp, ok := pat.(*ast.TypedPattern); ok {
		pat, typ = tp.Pat, tp.Type
	}
	id, ok := pat.(*ast.IdentPattern)
	if !ok || id.Name == nil {
		return nil, nil
	}
	name := id.Name.Text(c.file)
	s := span(id)
	if typ == nil {
		typ = literalType(b.Value, s)
	}
	if typ == nil {
		c.errorf(id.Pos(), "an @Observable property's type is written, or its value is a literal: 'var %s: T = …'", name)
		return nil, nil
	}
	signal := &ast.MemberExpr{Span: s, X: mkPath(s, "state"), Dot: s.Lo,
		Name: &ast.Ident{Span: s, Synth: "Signal"},
		Args: &ast.GenericArgs{Span: s, Lt: s.Lo, Args: []ast.Type{typ}, Gt: s.Hi}}
	var init ast.Expr
	if b.Value != nil {
		init = mkCall(signal, span(b.Value), b.Value)
	} else {
		init = mkCall(signal, s)
	}
	storageName := "$" + name
	// Its type written, so its value is checked with the bodies, where
	// the module's functions are declared.
	signalType := &ast.MemberType{Span: s, X: &ast.IdentType{Span: s, Name: &ast.Ident{Span: s, Synth: "state"}}, Dot: s.Lo,
		Name: &ast.Ident{Span: s, Synth: "Signal"}, Args: &ast.GenericArgs{Span: s, Lt: s.Lo, Args: []ast.Type{typ}, Gt: s.Hi}}
	storage := &ast.VarDecl{Span: v.Span, Mods: v.Mods, Keyword: v.Keyword, Kind: token.VAR,
		Bindings: []*ast.PatternBinding{{Span: b.Span, Assign: s.Lo, Value: init,
			Pat: &ast.TypedPattern{Span: s, Type: signalType,
				Pat: &ast.IdentPattern{Span: s, Name: &ast.Ident{Span: s, Synth: storageName}}}}}}
	value := func() ast.Expr {
		return &ast.MemberExpr{Span: s, X: mkPath(s, storageName), Dot: s.Lo, Name: &ast.Ident{Span: s, Synth: "Value"}}
	}
	accessor := func(kw string, st ast.Stmt) *ast.Accessor {
		return &ast.Accessor{Span: s, Keyword: &ast.Ident{Span: s, Synth: kw},
			Body: &ast.CodeBlock{Span: s, Lbrace: s.Lo, Stmts: []ast.Stmt{st}, Rbrace: s.Hi}}
	}
	get := accessor("get", &ast.ReturnStmt{Span: s, Return: s.Lo, X: value()})
	set := accessor("set", &ast.ExprStmt{Span: s, X: &ast.SequenceExpr{Span: s, Elements: []ast.Expr{
		value(),
		&ast.OperatorExpr{Span: s, Kind: token.ASSIGN, Synth: "="},
		mkPath(s, "newValue"),
	}}})
	prop := &ast.VarDecl{Span: v.Span, Attrs: v.Attrs, Mods: v.Mods, Keyword: v.Keyword, Kind: token.VAR,
		Bindings: []*ast.PatternBinding{{Span: b.Span,
			Pat:       &ast.TypedPattern{Span: s, Pat: &ast.IdentPattern{Span: s, Name: id.Name}, Type: typ},
			Accessors: &ast.AccessorBlock{Span: s, Lbrace: s.Lo, Accessors: []*ast.Accessor{get, set}, Rbrace: s.Hi}}}}
	return storage, prop
}

// literalType is the type a literal initial value has with nothing else
// said -- `"all"` a String, `0` an Int -- or nil.
func literalType(x ast.Expr, at ast.Span) ast.Type {
	name := ""
	switch x := x.(type) {
	case *ast.StringLit:
		name = "string"
	case *ast.BasicLit:
		switch x.Kind {
		case token.INT_LIT:
			name = "int"
		case token.FLOAT_LIT:
			name = "float64"
		case token.TRUE, token.FALSE:
			name = "bool"
		}
	}
	if name == "" {
		return nil
	}
	return &ast.IdentType{Span: at, Name: &ast.Ident{Span: at, Synth: name}}
}
