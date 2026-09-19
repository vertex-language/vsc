package analyzer

import (
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/token"
	"github.com/vertex-language/vsc/types"
)

// declareProtocol registers a protocol declaration, its Self parameter, and associated type names.
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

	// Inner scope contains Self and dependent associated types.
	inner := NewScope(scope, d.Pos(), d.End())
	inner.members = true
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

// resolveProtocol resolves inherited protocols and requirements within the protocol scope.
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
			sig := c.buildFuncSig(m.Sig, inner)
			unlabelOperator(m.Name.Text(c.file), sig)
			pr.Requirements = append(pr.Requirements, &types.Requirement{
				Name:       m.Name.Text(c.file),
				Sig:        sig,
				IsStatic:   isStatic(m.Mods),
				IsMutating: c.isMutating(m.Mods),
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

	// Explicit typealiases take precedence.
	if body != nil {
		for _, mem := range body.Members {
			alias, ok := mem.(*ast.TypealiasDecl)
			if !ok || alias.Name == nil || alias.Type == nil {
				continue
			}
			chosen[alias.Name.Text(c.file)] = c.resolveType(alias.Type, inner)
		}
	}

	// Infer unassigned associated types from requirement implementations.
	for _, p := range allProtocols(conformances) {
		for _, a := range p.Associated {
			if a == nil || chosen[a.Name] != nil {
				continue
			}
			if answer := inferAssociated(p, a.Name, fields, methods); answer != nil {
				chosen[a.Name] = answer
			}
		}
		// A Sequence's Element is what its iterator's next() answers, and
		// its Iterator the type makeIterator() makes -- or the type
		// itself, where it is its own iterator.
		if p.Name == "Sequence" && c.info.CoreTypes[p] {
			if it := c.iteration(t); it != nil {
				if chosen["Element"] == nil {
					chosen["Element"] = it.Element
				}
				if chosen["Iterator"] == nil {
					chosen["Iterator"] = it.Iterator
				}
			}
		}
	}
	if len(chosen) == 0 {
		return
	}
	*assoc = chosen
}

// inferAssociated infers an associated type by matching implementation signatures against protocol requirements.
func inferAssociated(p *types.Protocol, name string, fields []*types.Field, methods []*types.Method) types.Type {
	for _, req := range p.Requirements {
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

// matchDependent walks want alongside got, returning got's sub-type corresponding to dependent type name.
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
	case *types.Set:
		if g, ok := got.(*types.Set); ok {
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

// conformanceParts returns conformance list, fields, methods, and assoc map for nominal type t.
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

// allProtocols returns the transitive closure of protocols including all inherited protocols.
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

// resolveAssociatedTypes records every declared type's associated type choices.
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

// throughParam rewrites protocol Self references in type t to match stands.
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

// associatedConstraints returns all constraints declared or promised for an associated type.
func associatedConstraints(dep *types.Dependent) []types.Type {
	tp, ok := dep.Base.(*types.TypeParam)
	if !ok {
		return nil
	}
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

// applyWhere applies generic where-clause constraints and same-type requirements to type parameters.
func (c *checker) applyWhere(w *ast.GenericWhereClause, scope *Scope) {
	if w == nil {
		return
	}
	for _, req := range w.Reqs {
		switch r := req.(type) {
		case *ast.SameTypeReq:
			// `Element == String`: the parameter itself is that type here.
			if id, ok := r.Left.(*ast.IdentType); ok && id.Name != nil {
				if tn, ok := scope.Lookup(id.Name.Text(c.file)).(*TypeNameSymbol); ok {
					if tp, ok := tn.Type().(*types.TypeParam); ok {
						tp.Same = c.resolveType(r.Right, scope)
						continue
					}
				}
			}
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

// dependentOf extracts the type parameter and member name from a member type (e.g. C.Item).
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
