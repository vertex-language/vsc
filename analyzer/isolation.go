package analyzer

import (
	"strings"

	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/types"
)

// Isolation checking for @MainActor declarations and actor-isolated members.
// Synchronous @MainActor code must be invoked from @MainActor contexts or via await.

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

// declIsolated reports whether a declaration is @MainActor isolated.
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
	if f.Recv == nil && c.isVSXComponent(f, name) {
		return true
	}
	return name == "main" && f.Recv == nil && c.importing == ""
}

// isVSXComponent reports whether a function is a .vsx file's component:
// capitalized and returning Element (or Node). A component renders on the
// UI thread, so it is @MainActor, and may use what is.
func (c *checker) isVSXComponent(f *ast.FuncDecl, name string) bool {
	if c.file == nil || !strings.HasSuffix(c.file.Name(), ".vsx") || !isComponentName(name) || strings.Contains(name, ".") {
		return false
	}
	if f.Sig == nil || f.Sig.Result == nil {
		return false
	}
	id, ok := f.Sig.Result.Type.(*ast.IdentType)
	if !ok || id.Name == nil || id.Args != nil {
		return false
	}
	switch id.Name.Text(c.file) {
	case "Element", "Node":
		return true
	}
	return false
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

// inheritIsolation gives a class its superclass's isolation, as Swift
// does: a subclass of a @MainActor class is @MainActor. A @MainActor class
// whose superclass is not is refused, as Swift refuses it: its overrides
// would run off the main actor wherever the superclass is used.
func (c *checker) inheritIsolation(d *ast.ClassDecl, cl *types.Class) {
	super := classOf(cl.Superclass)
	if super == nil {
		return
	}
	superIsolated := c.classIsolated(super, map[*types.Class]bool{})
	switch {
	case superIsolated:
		c.info.MainActor[cl] = true
	case c.info.MainActor[cl] && d.Name != nil:
		c.errorf(d.Name.Pos(), "main actor-isolated class '%s' has different actor isolation from nonisolated superclass '%s'",
			d.Name.Text(c.file), super.Name)
	}
}

// classIsolated reports whether a class is @MainActor: by its attribute
// or its superclass's, here; an imported one by its initializers, which a
// @MainActor class's are.
func (c *checker) classIsolated(cl *types.Class, seen map[*types.Class]bool) bool {
	if cl == nil || seen[cl] {
		return false
	}
	seen[cl] = true
	if c.info.MainActor[cl] {
		return true
	}
	for _, sig := range cl.Inits {
		if sig != nil && sig.Isolated {
			return true
		}
	}
	return c.classIsolated(classOf(cl.Superclass), seen)
}

// classOf is the class a superclass type names, generic or not.
func classOf(t types.Type) *types.Class {
	if t == nil {
		return nil
	}
	cl, _ := t.Underlying().(*types.Class)
	return cl
}
