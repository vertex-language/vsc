package analyzer

import (
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/token"
	"github.com/vertex-language/vsc/types"
)

// A wrappedLocal is a local variable a property wrapper gives:
// `@Clamped(0...10) var level = 5` in a function body. Its storage is a
// local of the wrapper's type, `_level`; `level` reads and writes the
// storage's wrappedValue, and `$level` its projectedValue.
type wrappedLocal struct {
	storage string // the storage local's name, `_level`
	inner   string // "wrappedValue" or "projectedValue"
}

// A wrappedLocalDecl is what rewriteWrappedLocal learned of a declaration,
// kept so a body checked twice declares the same names again.
type wrappedLocalDecl struct {
	name  string    // `level`
	at    token.Pos // the wrapper attribute
	typed ast.Type  // the type written for the variable, if any
}

// rewriteWrappedLocal makes `@W(args) var x = v` in a function body its
// storage, `var _x = W(wrappedValue: v, args)`, in place, declares x (and
// $x where W has a projectedValue) as names that read through it, and
// reports whether it did.
func (c *checker) rewriteWrappedLocal(d *ast.VarDecl, scope *Scope) bool {
	w := c.wrappedLocalDecls[d]
	if w == nil {
		attr := c.wrapperAttr(d.Attrs, scope)
		if attr == nil {
			return false
		}
		if len(d.Bindings) != 1 || d.Bindings[0].Body != nil || d.Bindings[0].Accessors != nil {
			c.errorf(attr.Pos(), "property wrapper can only be applied to a single stored variable")
			return false
		}
		b := d.Bindings[0]
		var typed ast.Type
		pat := b.Pat
		if tp, ok := pat.(*ast.TypedPattern); ok {
			pat, typed = tp.Pat, tp.Type
		}
		id, ok := pat.(*ast.IdentPattern)
		if !ok || id.Name == nil {
			c.errorf(attr.Pos(), "property wrapper can only be applied to a single named variable")
			return false
		}
		if d.Kind != token.VAR {
			c.errorf(attr.Pos(), "property wrapper can only be applied to a 'var'")
			return false
		}
		// `@State var todos: [Todo] = []`: the type written is the
		// wrapped value's, which the initial value is read as.
		if typed != nil && b.Value != nil {
			at := ast.Span{Lo: b.Value.Pos(), Hi: b.Value.End()}
			b.Value = &ast.CastExpr{Span: at, X: b.Value, Keyword: at.Lo, Kind: token.AS, Type: typed}
		}
		call := c.wrapperInit(attr, b)
		if call == nil {
			c.errorf(attr.Pos(), "a local wrapped variable needs an initial value or wrapper arguments")
			return false
		}
		name := id.Name.Text(c.file)
		w = &wrappedLocalDecl{name: name, at: attr.Pos(), typed: typed}
		// The attribute goes, so a second check meets plain storage.
		for i, a := range d.Attrs {
			if a == attr {
				d.Attrs = append(d.Attrs[:i:i], d.Attrs[i+1:]...)
				break
			}
		}
		span := ast.Span{Lo: id.Pos(), Hi: id.End()}
		b.Pat = &ast.IdentPattern{Span: span, Name: &ast.Ident{Span: span, Synth: "_" + name}}
		b.Value = call
		if b.Assign == token.NoPos {
			b.Assign = call.Pos()
		}
		if c.wrappedLocalDecls == nil {
			c.wrappedLocalDecls = map[*ast.VarDecl]*wrappedLocalDecl{}
		}
		c.wrappedLocalDecls[d] = w
	}

	b := d.Bindings[0]
	c.checkStored(b, false, scope)
	storage, _ := c.lookupValue(scope, "_"+w.name).(*VarSymbol)
	if storage == nil || isInvalid(storage.Type()) {
		return true
	}
	if c.wrappedLocals == nil {
		c.wrappedLocals = map[*VarSymbol]*wrappedLocal{}
	}
	declare := func(name, inner string) bool {
		f := wrapperMember(storage.Type(), inner)
		if f == nil {
			return false
		}
		at := ast.Span{Lo: w.at, Hi: w.at}
		read := &ast.MemberExpr{Span: at, X: ident("_"+w.name, at), Dot: w.at, Name: &ast.Ident{Span: at, Synth: inner}}
		t := c.checkExpr(read, nil, scope)
		settable := !f.IsConst && (!f.IsComputed || f.HasSetter)
		v := NewVar(name, t, w.at, !settable, types.DefaultOwnership)
		scope.Insert(v)
		c.wrappedLocals[v] = &wrappedLocal{storage: "_" + w.name, inner: inner}
		return true
	}
	if !declare(w.name, "wrappedValue") {
		c.errorf(w.at, "property wrapper '%s' has no 'wrappedValue'", storage.Type())
		return true
	}
	if w.typed != nil {
		want := c.resolveType(w.typed, scope)
		if got, _ := c.lookupValue(scope, w.name).(*VarSymbol); got != nil && want != nil &&
			!isInvalid(want) && !c.assignableTo(got.Type(), want) {
			c.typeErrorf(w.at, "property wrapper's wrappedValue is '%s', not the declared '%s'", got.Type(), want)
		}
	}
	declare("$"+w.name, "projectedValue")
	return true
}

// wrappedLocalRead checks a use of a wrapped local's name as the storage's
// wrappedValue or projectedValue, which is what is lowered in its place.
// The name is recorded as a use of the storage, so a closure that names it
// captures the storage.
func (c *checker) wrappedLocalRead(e *ast.IdentExpr, w *wrappedLocal, expected types.Type, scope *Scope) types.Type {
	at := ast.Span{Lo: e.Pos(), Hi: e.End()}
	read := &ast.MemberExpr{Span: at, X: ident(w.storage, at), Dot: e.Pos(), Name: &ast.Ident{Span: at, Synth: w.inner}}
	t := c.checkExpr(read, expected, scope)
	c.info.ImplicitSelf[e] = read
	if s := c.lookupValue(scope, w.storage); s != nil {
		c.info.Uses[e.Name] = s
	}
	return t
}
