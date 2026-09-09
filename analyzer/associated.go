package analyzer

import (
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/token"
	"github.com/vertex-language/vsc/types"
)

// Associated types: the ones a protocol names and a conformer chooses.
//
//	protocol Container {
//	    associatedtype Item
//	    func get() -> Item
//	}
//
// Item is not a type. It is a name for whichever type the conforming
// type decides it is, and every conformer may decide differently --
// which is what makes a protocol a description of a family of types
// rather than of one. `Self` is the same idea one level up: the
// conforming type itself, named from inside the protocol.
//
// So a protocol is generic over Self, and an associated type is a
// type reached through it. Both are modelled that way: Self is a type
// parameter, and `Item` inside the protocol is `Self.Item` -- a
// types.Dependent, which is a dependency rather than an answer.
//
// # Where the answer comes from
//
// The conformer, in one of two ways. It may say so outright with a
// typealias, and it may say so by writing the implementation:
//
//	struct Ints: Container {
//	    func get() -> Int32 { return 7 }   // so Item is Int32
//	}
//
// swiftc infers it the second way and most Swift code relies on that,
// so this does too. The inference is a match of the implementation's
// signature against the requirement's, position by position: where
// the requirement says `Self.Item` and the implementation says Int32,
// Item is Int32. A conformer that says nothing either way leaves the
// choice unmade, and the conformance is refused by name rather than
// completed with a guess.
//
// # What it is for
//
// Substitution. `C.Item` with C standing for Ints is Ints's choice,
// which is what makes a generic over a protocol lowerable: the
// specializer already replaces C, and this is what it replaces
// `C.Item` with. See types.AssocOf and vil/gen/generic.go.

// declareProtocol makes the protocol, its Self, and the names its own
// body will be read in.
//
// It happens in the pass that declares types rather than the one that
// reads their members, because a protocol's name and the names of its
// associated types are both things another declaration may mention
// before this one is read.
func (c *checker) declareProtocol(d *ast.ProtocolDecl, scope *Scope) {
	if d.Name == nil {
		return
	}
	name := d.Name.Text(c.file)
	pr := &types.Protocol{Name: name}
	pr.Self = &types.TypeParam{Name: "Self", Constraints: []types.Type{pr}}

	sym := NewTypeName(name, pr, d.Name.Pos())
	sym.SetDecl(d)
	if old := scope.Insert(sym); old != nil {
		c.errorf(d.Name.Pos(), "invalid redeclaration of '%s'", name)
	}
	c.info.Defs[d.Name] = sym
	c.declaredHere(sym, c.accessOf(d.Mods))

	// The scope the requirements are read in: Self, and one name per
	// associated type. Both are dependencies rather than types, and
	// putting them here is what lets `func get() -> Item` resolve at
	// all.
	inner := NewScope(scope, d.Pos(), d.End())
	c.info.Scopes[d] = inner
	if c.typeScopes == nil {
		c.typeScopes = map[string]*Scope{}
	}
	c.typeScopes[name] = inner
	inner.Insert(NewTypeName("Self", pr.Self, d.Name.Pos()))

	if d.Body == nil {
		return
	}
	for _, mem := range d.Body.Members {
		at, ok := mem.(*ast.AssociatedTypeDecl)
		if !ok || at.Name == nil {
			continue
		}
		an := at.Name.Text(c.file)
		assoc := &types.Associated{Name: an}
		pr.Associated = append(pr.Associated, assoc)
		dep := &types.Dependent{Base: pr.Self, Name: an}
		tn := NewTypeName(an, dep, at.Name.Pos())
		inner.Insert(tn)
		c.info.Defs[at.Name] = tn
	}
}

// resolveProtocol reads what the protocol promises, in the scope
// declareProtocol prepared.
//
// This runs with the other types' members rather than with the
// declarations, so a requirement may name a type declared further
// down the file -- which is Swift's rule, and was not true of
// protocols here until associated types made a second pass necessary
// anyway.
func (c *checker) resolveProtocol(d *ast.ProtocolDecl, scope *Scope) {
	if d.Name == nil {
		return
	}
	sym, _ := scope.Lookup(d.Name.Text(c.file)).(*TypeNameSymbol)
	if sym == nil {
		return
	}
	pr, ok := sym.Type().(*types.Protocol)
	if !ok {
		return
	}
	inner := c.typeScopes[d.Name.Text(c.file)]
	if inner == nil {
		inner = scope
	}

	if d.Inherit != nil {
		for _, item := range d.Inherit.Items {
			if up, ok := c.resolveType(item.Type, scope).(*types.Protocol); ok {
				pr.Inherited = append(pr.Inherited, up)
			}
		}
	}
	if d.Body == nil {
		return
	}
	byName := map[string]*types.Associated{}
	for _, a := range pr.Associated {
		byName[a.Name] = a
	}
	for _, mem := range d.Body.Members {
		switch m := mem.(type) {
		case *ast.AssociatedTypeDecl:
			// What the conformer's choice has to satisfy. It is read
			// here rather than where the name was declared because a
			// constraint is a protocol, and that protocol may be
			// declared below this one.
			if m.Name == nil || m.Inherit == nil {
				continue
			}
			assoc := byName[m.Name.Text(c.file)]
			if assoc == nil {
				continue
			}
			for _, item := range m.Inherit.Items {
				if p, ok := c.resolveType(item.Type, inner).(*types.Protocol); ok {
					assoc.Constraints = append(assoc.Constraints, p)
				}
			}

		case *ast.FuncDecl:
			if m.Name == nil {
				continue
			}
			pr.Requirements = append(pr.Requirements, &types.Requirement{
				Name: m.Name.Text(c.file),
				Sig:  c.buildFuncSig(m.Sig, inner),
			})

		case *ast.VarDecl:
			for _, b := range m.Bindings {
				tp, ok := b.Pat.(*ast.TypedPattern)
				if !ok {
					continue
				}
				id, ok := tp.Pat.(*ast.IdentPattern)
				if !ok || id.Name == nil {
					continue
				}
				pr.Requirements = append(pr.Requirements, &types.Requirement{
					Name:    id.Name.Text(c.file),
					Type:    c.resolveType(tp.Type, inner),
					IsVar:   m.Kind == token.VAR,
					IsConst: m.Kind == token.LET,
				})
			}
		}
	}
}

// resolveAssociatedChoices records what a type chose for each
// associated type its protocols name.
//
// The typealias it wrote comes first: a conformer that says outright
// what Item is has said it, and nothing inferred may contradict it.
// What is left is read out of the implementations, which is how most
// Swift code says it and how swiftc reads it.
func (c *checker) resolveAssociatedChoices(t types.Type, body *ast.MemberBlock, inner *Scope) {
	conformances, fields, methods, assoc := conformanceParts(t)
	if len(conformances) == 0 {
		return
	}
	chosen := map[string]types.Type{}

	// Said outright.
	if body != nil {
		for _, mem := range body.Members {
			alias, ok := mem.(*ast.TypealiasDecl)
			if !ok || alias.Name == nil || alias.Type == nil {
				continue
			}
			chosen[alias.Name.Text(c.file)] = c.resolveType(alias.Type, inner)
		}
	}

	// Read out of the implementations.
	for _, p := range allProtocols(conformances) {
		for _, a := range p.Associated {
			if a == nil || chosen[a.Name] != nil {
				continue
			}
			if answer := inferAssociated(p, a.Name, fields, methods); answer != nil {
				chosen[a.Name] = answer
			}
		}
	}
	if len(chosen) == 0 {
		return
	}
	*assoc = chosen
}

// inferAssociated is the type an implementation implies for one
// associated type, or nil where nothing in it says.
//
// One requirement at a time, matched against the method of the same
// name: wherever the requirement's signature says this associated
// type, the implementation's signature says what it is. The first
// answer wins, and a second that disagrees is not a different answer
// -- it is a conformance that does not hold, which checkConformance
// reports when it compares the signatures it now has.
func inferAssociated(p *types.Protocol, name string, fields []*types.Field, methods []*types.Method) types.Type {
	for _, req := range p.Requirements {
		// A property requirement says it as plainly as a method does:
		// `var head: Item` implemented as `var head: Int32` is a
		// conformer saying Item is Int32.
		if req.Type != nil {
			for _, f := range fields {
				if f == nil || f.Name != req.Name {
					continue
				}
				if answer := matchDependent(req.Type, f.Type, name); answer != nil {
					return answer
				}
			}
			continue
		}
		if req.Sig == nil {
			continue
		}
		for _, m := range methods {
			if m.Name != req.Name || m.Sig == nil {
				continue
			}
			if answer := matchDependent(req.Sig, m.Sig, name); answer != nil {
				return answer
			}
		}
	}
	return nil
}

// matchDependent walks a requirement's type beside an
// implementation's and returns what the implementation has where the
// requirement has this associated type.
func matchDependent(want, got types.Type, name string) types.Type {
	if want == nil || got == nil {
		return nil
	}
	if dep, ok := want.(*types.Dependent); ok && dep.Name == name {
		return got
	}
	switch w := want.(type) {
	case *types.Signature:
		g, ok := got.(*types.Signature)
		if !ok || len(w.Params) != len(g.Params) {
			return nil
		}
		for i, p := range w.Params {
			if answer := matchDependent(p.Type, g.Params[i].Type, name); answer != nil {
				return answer
			}
		}
		return matchDependent(w.Results, g.Results, name)
	case *types.Optional:
		if g, ok := got.(*types.Optional); ok {
			return matchDependent(w.Wrapped, g.Wrapped, name)
		}
	case *types.Array:
		if g, ok := got.(*types.Array); ok {
			return matchDependent(w.Elem, g.Elem, name)
		}
	case *types.Dictionary:
		if g, ok := got.(*types.Dictionary); ok {
			if answer := matchDependent(w.Key, g.Key, name); answer != nil {
				return answer
			}
			return matchDependent(w.Value, g.Value, name)
		}
	case *types.Tuple:
		g, ok := got.(*types.Tuple)
		if !ok || len(w.Elements) != len(g.Elements) {
			return nil
		}
		for i, e := range w.Elements {
			if answer := matchDependent(e.Type, g.Elements[i].Type, name); answer != nil {
				return answer
			}
		}
	}
	return nil
}

// conformanceParts is what an associated type's answer is read from
// and written to.
func conformanceParts(t types.Type) ([]*types.Protocol, []*types.Field, []*types.Method, *map[string]types.Type) {
	switch n := t.(type) {
	case *types.Struct:
		return n.Conformances, n.Fields, n.Methods, &n.Assoc
	case *types.Class:
		return n.Conformances, n.Fields, n.Methods, &n.Assoc
	case *types.Enum:
		return n.Conformances, nil, n.Methods, &n.Assoc
	}
	return nil, nil, nil, nil
}

// allProtocols is a conformance list with everything those protocols
// inherit, so that an associated type declared by an inherited
// protocol is answered too.
func allProtocols(list []*types.Protocol) []*types.Protocol {
	var out []*types.Protocol
	seen := map[*types.Protocol]bool{}
	var add func(p *types.Protocol)
	add = func(p *types.Protocol) {
		if p == nil || seen[p] {
			return
		}
		seen[p] = true
		out = append(out, p)
		for _, up := range p.Inherited {
			add(up)
		}
	}
	for _, p := range list {
		add(p)
	}
	return out
}

// resolveAssociatedTypes records every declared type's choices.
func (c *checker) resolveAssociatedTypes(decls []ast.Decl, scope *Scope) {
	for _, d := range decls {
		var name *ast.Ident
		var body *ast.MemberBlock
		switch d := d.(type) {
		case *ast.StructDecl:
			name, body = d.Name, d.Body
		case *ast.ClassDecl:
			name, body = d.Name, d.Body
		case *ast.ActorDecl:
			name, body = d.Name, d.Body
		case *ast.EnumDecl:
			name, body = d.Name, d.Body
		default:
			continue
		}
		if name == nil {
			continue
		}
		sym, _ := scope.Lookup(name.Text(c.file)).(*TypeNameSymbol)
		if sym == nil {
			continue
		}
		inner := c.typeScopes[name.Text(c.file)]
		if inner == nil {
			inner = scope
		}
		c.resolveAssociatedChoices(sym.Type(), body, inner)
	}
}

// throughParam rewrites what a protocol promises so that it is said
// about whatever promised it.
//
// A requirement declared as `func get() -> Self.Item` promises, of a
// parameter `C: Container`, a `C.Item`. Handing back the requirement's
// own spelling instead makes `return c.get()` a return of Self.Item
// where C.Item was expected -- two names for one type, reported as a
// mismatch.
//
// Every protocol in the hierarchy is rewritten, not just the one that
// declared the requirement: an inherited protocol has a Self of its
// own, and it stands for the same type.
func throughParam(con types.Type, stands types.Type, t types.Type) types.Type {
	p, ok := con.(*types.Protocol)
	if !ok || t == nil || stands == nil {
		return t
	}
	subst := map[*types.TypeParam]types.Type{}
	for _, up := range allProtocols([]*types.Protocol{p}) {
		if up.Self != nil {
			subst[up.Self] = stands
		}
	}
	if len(subst) == 0 {
		return t
	}
	return types.Substitute(t, subst)
}

// associatedConstraints is what the associated type a dependent
// member type names was declared to satisfy.
//
// `associatedtype Item: Number` is the only thing known about Item
// before something says what it is, and it is what makes
// `c.get().value()` check inside a generic body.
func associatedConstraints(dep *types.Dependent) []types.Type {
	tp, ok := dep.Base.(*types.TypeParam)
	if !ok {
		return nil
	}
	// What a `where` clause promised about this one, first: it is the
	// more specific statement, and the one written closest to the use.
	out := append([]types.Type(nil), tp.Promised[dep.Name]...)
	for _, con := range tp.Constraints {
		p, ok := con.(*types.Protocol)
		if !ok {
			continue
		}
		for _, up := range allProtocols([]*types.Protocol{p}) {
			for _, a := range up.Associated {
				if a == nil || a.Name != dep.Name {
					continue
				}
				for _, ac := range a.Constraints {
					out = append(out, ac)
				}
			}
		}
	}
	return out
}

// applyWhere reads a `where` clause into the parameters it is about.
//
// Two forms are read. `where C: Container` is the constraint the
// angle brackets could have carried, and goes where that one goes.
// `where C.Item == Int32` and `where C.Item: Number` are about the
// parameter's associated types, which the angle brackets cannot say
// at all -- and are the reason the clause exists.
//
// It runs after the parameters are declared, because both sides may
// name one.
func (c *checker) applyWhere(w *ast.GenericWhereClause, scope *Scope) {
	if w == nil {
		return
	}
	for _, req := range w.Reqs {
		switch r := req.(type) {
		case *ast.SameTypeReq:
			tp, name, ok := c.dependentOf(r.Left, scope)
			if !ok {
				continue
			}
			if tp.Bound == nil {
				tp.Bound = map[string]types.Type{}
			}
			tp.Bound[name] = c.resolveType(r.Right, scope)

		case *ast.ConformanceReq:
			right := c.resolveType(r.Right, scope)
			if right == nil {
				continue
			}
			// `where C: P` -- the same thing the angle brackets say.
			if id, ok := r.Left.(*ast.IdentType); ok && id.Name != nil {
				if tn, ok := scope.Lookup(id.Name.Text(c.file)).(*TypeNameSymbol); ok {
					if tp, ok := tn.Type().(*types.TypeParam); ok {
						tp.Constraints = append(tp.Constraints, right)
						continue
					}
				}
			}
			tp, name, ok := c.dependentOf(r.Left, scope)
			if !ok {
				continue
			}
			if tp.Promised == nil {
				tp.Promised = map[string][]types.Type{}
			}
			tp.Promised[name] = append(tp.Promised[name], right)
		}
	}
}

// dependentOf reads the `C.Item` on the left of a where requirement:
// the parameter, and the associated type's name.
func (c *checker) dependentOf(t ast.Type, scope *Scope) (*types.TypeParam, string, bool) {
	mem, ok := t.(*ast.MemberType)
	if !ok || mem.Name == nil {
		return nil, "", false
	}
	id, ok := mem.X.(*ast.IdentType)
	if !ok || id.Name == nil {
		return nil, "", false
	}
	tn, ok := scope.Lookup(id.Name.Text(c.file)).(*TypeNameSymbol)
	if !ok {
		return nil, "", false
	}
	tp, ok := tn.Type().(*types.TypeParam)
	if !ok {
		return nil, "", false
	}
	return tp, mem.Name.Text(c.file), true
}
