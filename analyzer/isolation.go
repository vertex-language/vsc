package analyzer

import (
	"strings"

	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/types"
)

// Isolation: what @MainActor means, checked as Swift checks it.
//
// A function, initializer, property or type may be marked @MainActor; a
// type's members are then @MainActor too, in its body and in its
// extensions, unless one says `nonisolated`. Top-level code, `main` and
// what is nested in @MainActor code -- local functions and closures --
// run on the main thread as well. The rule for the rest of the program
// is Swift's for actor-isolated declarations: synchronous @MainActor code
// is called from @MainActor code, or from an async function under
// `await`, which is what gets the task to the main thread and back; an
// async @MainActor function is called with `await` from anywhere, and
// gets itself there.

// mainActorAttr is the attribute's spelling.
const mainActorAttr = "MainActor"

// hasAttr reports whether one of the attributes is @name.
func (c *checker) hasAttr(attrs []*ast.Attr, name string) bool {
	for _, a := range attrs {
		if a == nil {
			continue
		}
		if id, ok := a.Name.(*ast.IdentType); ok && id.Name != nil && id.Name.Text(c.file) == name {
			return true
		}
	}
	return false
}

// declIsolated is whether a declaration with these attributes and
// modifiers is @MainActor: marked so itself, or a member of a type that
// is, unless it is `nonisolated`.
func (c *checker) declIsolated(attrs []*ast.Attr, mods []*ast.Modifier) bool {
	if c.hasModifier(mods, "nonisolated") {
		return false
	}
	return c.memberIsolated || c.hasAttr(attrs, mainActorAttr)
}

// funcIsolated is whether a function declared in scope is @MainActor:
// by its attribute; as `main`, which is where top-level code runs; or,
// for one local to a body, as its surroundings are, since a local
// function runs where the code around it does, as Swift's does.
func (c *checker) funcIsolated(f *ast.FuncDecl, name string, scope *Scope) bool {
	if c.hasModifier(f.Mods, "nonisolated") {
		return false
	}
	if c.hasAttr(f.Attrs, mainActorAttr) {
		return true
	}
	if scope != c.pkgScope {
		return c.currIsolated
	}
	return name == "main" && f.Recv == nil && c.importing == ""
}

// markIsolatedVars records @MainActor on the variables a declaration
// binds, once they are declared.
func (c *checker) markIsolatedVars(d *ast.VarDecl) {
	if !c.hasAttr(d.Attrs, mainActorAttr) {
		return
	}
	for _, b := range d.Bindings {
		pat := b.Pat
		if tp, ok := pat.(*ast.TypedPattern); ok {
			pat = tp.Pat
		}
		if id, ok := pat.(*ast.IdentPattern); ok && id.Name != nil {
			if v, ok := c.info.Defs[id.Name].(*VarSymbol); ok {
				v.isolated = true
			}
		}
	}
}

// checkIsolatedCall holds a call to a @MainActor function to Swift's
// rule for a call to an actor-isolated one, in Swift's words. From
// @MainActor code any call is fine. From elsewhere, an async one needs
// `await`, which checkAsyncCall asks for; a synchronous one is a call
// that suspends to get there, so it needs `await` too and a place that
// can suspend. what is Swift's word for the declaration: "instance
// method", "global function", "initializer".
func (c *checker) checkIsolatedCall(call ast.Expr, sig *types.Signature, what, name string) {
	if sig == nil || !sig.Isolated || sig.Async || c.currIsolated {
		return
	}
	switch {
	case !c.currAsync:
		c.errorf(call.Pos(), "call to main actor-isolated %s '%s' in a synchronous nonisolated context", what, name)
	case !c.inAwait:
		c.errorf(call.Pos(), "main actor-isolated %s '%s' cannot be called from outside of the actor", what, name)
	}
}

// checkIsolatedAccess holds a use of a @MainActor property or variable,
// from code that is not, to the same rule: read under `await` where the
// code can suspend, and never written, since a write cannot wait. what
// is Swift's word for it: "property" or "var".
func (c *checker) checkIsolatedAccess(at ast.Expr, what, name string, write bool) {
	if c.currIsolated {
		return
	}
	switch {
	case write:
		c.errorf(at.Pos(), "main actor-isolated %s '%s' can not be mutated from a nonisolated context", what, name)
	case !c.currAsync:
		c.errorf(at.Pos(), "main actor-isolated %s '%s' can not be referenced from a nonisolated context", what, name)
	case !c.inAwait:
		c.errorf(at.Pos(), "main actor-isolated %s '%s' cannot be accessed from outside of the actor", what, name)
	}
}

// calleeWords is how Swift names what a call calls in a diagnostic: the
// kind of declaration and its name with its labels, `add(x:y:)`.
func (c *checker) calleeWords(call *ast.CallExpr, sig *types.Signature) (what, name string) {
	what = "function"
	switch f := call.Fun.(type) {
	case *ast.IdentExpr:
		if f.Name != nil {
			name = f.Name.Text(c.file)
		}
		what = "global function"
		if _, isType := c.info.Types[f].(*types.Metatype); isType {
			what, name = "initializer", "init"
		}
	case *ast.MemberExpr:
		if f.Name != nil {
			name = f.Name.Text(c.file)
		}
		what = "instance method"
		if _, isType := c.info.Types[f.X].(*types.Metatype); isType {
			what = "static method"
			if name == "init" {
				what = "initializer"
			}
		}
	}
	if sig == nil {
		return what, name
	}
	var b strings.Builder
	b.WriteString(name)
	b.WriteByte('(')
	for _, p := range sig.Params {
		if p.Label == "" {
			b.WriteByte('_')
		} else {
			b.WriteString(p.Label)
		}
		b.WriteByte(':')
	}
	b.WriteByte(')')
	return what, b.String()
}

// IsolatedField reports whether t has a @MainActor property of the name:
// stored, computed or static.
func IsolatedField(t types.Type, name string) bool {
	if t == nil {
		return false
	}
	if meta, ok := t.(*types.Metatype); ok {
		t = meta.Instance
	}
	var fields, computed, statics []*types.Field
	switch u := t.Underlying().(type) {
	case *types.Struct:
		fields, computed, statics = u.Fields, u.Computed, u.Statics
	case *types.Class:
		fields, computed, statics = u.Fields, u.Computed, u.Statics
	case *types.Enum:
		computed, statics = u.Computed, u.Statics
	default:
		return false
	}
	for _, list := range [][]*types.Field{fields, computed, statics} {
		for _, f := range list {
			if f != nil && f.Name == name {
				return f.Isolated
			}
		}
	}
	return false
}
