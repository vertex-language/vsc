package gen

import (
	"strings"

	"github.com/vertex-language/vsc/analyzer"
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/internal/sil"
	"github.com/vertex-language/vsc/mangle"
	"github.com/vertex-language/vsc/types"
)

// A computed property of a generic type is lowered as its methods are:
// for each instance it is read or written on, with the type's parameters
// standing for the instance's arguments. Its accessors have no symbol
// until then.

// genericProperty is where a computed property of a generic type was
// written, in the type's body or an extension of it, and the linkage
// its declaration asks for.
type genericProperty struct {
	binding *ast.PatternBinding
	linkage sil.Linkage
}

// genericPropertyDecl finds the declaration of a generic type's computed
// property.
func (g *gen) genericPropertyDecl(base types.Type, name string) (genericProperty, bool) {
	if g.props == nil {
		g.props = map[genericMethodKey]genericProperty{}
		add := func(t types.Type, body *ast.MemberBlock, file *ast.File) {
			// A protocol's extension's are written in terms of Self, and
			// lowered for each conformer as a generic type's are for each
			// instance.
			_, isProtocol := t.(*types.Protocol)
			if t == nil || body == nil || (len(nominalTypeParams(t)) == 0 && builtinParams(g.info, t) == nil && !isProtocol) {
				return
			}
			prev := g.file
			g.file = file.Unit
			defer func() { g.file = prev }()
			for _, mem := range body.Members {
				vd, ok := mem.(*ast.VarDecl)
				if !ok || isStaticDecl(vd.Mods) {
					continue
				}
				for _, b := range vd.Bindings {
					n := g.computedName(b)
					key := genericMethodKey{typ: t, name: n}
					if _, seen := g.props[key]; n != "" && !seen && g.getterBody(b) != nil {
						g.props[key] = genericProperty{binding: b, linkage: g.accessLinkage(vd.Mods)}
					}
				}
			}
		}
		declared := func(name *ast.Ident) types.Type {
			if name == nil {
				return nil
			}
			if s, ok := g.info.Defs[name].(*analyzer.TypeNameSymbol); ok {
				return s.Type()
			}
			return nil
		}
		for _, f := range g.files {
			for _, stmt := range f.Stmts {
				decl, ok := stmt.(*ast.DeclStmt)
				if !ok {
					continue
				}
				switch d := decl.D.(type) {
				case *ast.StructDecl:
					add(declared(d.Name), d.Body, f)
				case *ast.ClassDecl:
					add(declared(d.Name), d.Body, f)
				case *ast.EnumDecl:
					add(declared(d.Name), d.Body, f)
				case *ast.ExtensionDecl:
					add(g.info.Extensions[d], d.Body, f)
				}
			}
		}
	}
	p, ok := g.props[genericMethodKey{typ: base, name: name}]
	return p, ok
}

// genericAccessor is the symbol of a generic type's getter or setter for
// the instance recv, lowering it the first time. generic is false where
// recv is not an instance of a generic type; an empty symbol with generic
// true means it was refused.
func (g *gen) genericAccessor(at ast.Node, recv types.Type, f *types.Field, setter bool) (string, types.Type, bool) {
	var base types.Type
	var params []*types.TypeParam
	var args []types.Type
	if inst, ok := recv.(*types.GenericInstance); ok {
		base, params, args = inst.Base, nominalTypeParams(inst.Base), inst.Args
	} else if b := builtinOf(g.info, recv); b != nil && len(b.Params) > 0 {
		// A property an extension gives Array, Dictionary, Set or Optional.
		base, params, args = b.Type, b.Params, b.Args(recv)
	}
	if len(params) == 0 {
		return "", nil, false
	}
	if len(args) != len(params) {
		g.refuse(at, "a property of a generic type on something whose type arguments are not known")
		return "", nil, true
	}
	decl, ok := g.genericPropertyDecl(base, f.Name)
	if !ok {
		g.refuse(at, "a computed property of a generic type whose declaration this cannot find")
		return "", nil, true
	}
	subst := make(map[*types.TypeParam]types.Type, len(g.subst)+len(params))
	for k, v := range g.subst {
		subst[k] = v
	}
	for i, p := range params {
		subst[p] = args[i]
	}
	t := types.Substitute(f.Type, subst)
	d := mangle.Decl{
		Module:    g.memberModule(base, f),
		Context:   memberChain(base),
		Extended:  extendedBuiltin(base),
		Name:      f.Name,
		Signature: &types.Signature{Results: t},
		ModuleOf:  g.moduleOfType,
	}
	naming := mangle.Getter
	if setter {
		naming = mangle.Setter
	}
	mangled, err := naming(d)
	if err != nil {
		g.refuse(at, "a property of a generic type this compiler cannot name: "+err.Error())
		return "", nil, true
	}
	var b strings.Builder
	b.WriteString(mangled)
	b.WriteString("Tv")
	for _, a := range args {
		b.WriteString(identifierSafe(a.String()))
	}
	name := b.String()

	if g.specialized[name] {
		return name, t, true
	}
	if existing := g.m.Lookup(name); existing != nil && !existing.IsDeclaration() {
		return name, t, true
	}
	var body *ast.CodeBlock
	var set *ast.Accessor
	if setter {
		if set = g.setterAccessor(decl.binding); set == nil {
			g.refuse(at, "an assignment to a computed property with no setter")
			return "", nil, true
		}
	} else {
		body = g.getterBody(decl.binding)
	}
	if g.specialized == nil {
		g.specialized = map[string]bool{}
	}
	g.specialized[name] = true

	restore := g.aside()
	endSpecialization := g.asSpecialization()
	defer endSpecialization()
	defer restore()
	if file := g.fileOf(decl.binding); file != nil {
		g.file = file
	}
	g.subst = subst
	// This module's own copy, whatever the declaration's access.
	if setter {
		g.emitSetterNamed(name, recv, f.Name, t, set, sil.Private)
	} else {
		g.emitGetterNamed(name, recv, f.Name, t, body, false, sil.Private)
	}
	return name, t, true
}

// aside saves the state of the function being lowered, so that another
// can be lowered in the middle of it, and returns what restores it.
func (g *gen) aside() func() {
	outer := struct {
		fn      *sil.Func
		entry   bool
		blk     *sil.Block
		scopes  []*scope
		locals  map[analyzer.Symbol]*local
		loops   []loop
		pending string
		recv    types.Type
		self    *local
		subst   map[*types.TypeParam]types.Type
		throws  bool
		catches []catchTarget
		tryBang bool
		tryCall *ast.CallExpr
	}{g.fn, g.entry, g.blk, g.scopes, g.locals, g.loops, g.pending, g.recv, g.self, g.subst,
		g.throws, g.catches, g.tryBang, g.tryCall}
	prevFile := g.file
	return func() {
		g.fn, g.entry, g.blk = outer.fn, outer.entry, outer.blk
		g.scopes, g.locals = outer.scopes, outer.locals
		g.loops, g.pending = outer.loops, outer.pending
		g.recv, g.self, g.subst = outer.recv, outer.self, outer.subst
		g.throws, g.catches, g.tryBang, g.tryCall = outer.throws, outer.catches, outer.tryBang, outer.tryCall
		g.file = prevFile
	}
}

// extensionProperty is the protocol whose extension gives t the computed
// property name, and the property, written in terms of the protocol's Self.
func (g *gen) extensionProperty(t types.Type, name string) (*types.Protocol, *types.Field) {
	for _, p := range g.conformancesOf(t) {
		if q, f := p.ExtensionProperty(name, false); f != nil {
			return q, f
		}
	}
	return nil, nil
}

// protocolAccessor is the symbol of the getter a protocol's extension
// gives recv, lowered for recv -- Self standing for it -- the first time,
// as a protocol extension's method is; see protocolExtensionMethod. ok is
// false where f is not such a property of recv; an empty symbol with ok
// true means it was refused.
func (g *gen) protocolAccessor(at ast.Node, recv types.Type, f *types.Field) (string, types.Type, bool) {
	p, pf := g.extensionProperty(recv, f.Name)
	if pf == nil || (pf != f && f.Origin != pf) || p.Self == nil {
		return "", nil, false
	}
	decl, ok := g.genericPropertyDecl(p, pf.Name)
	if !ok {
		g.refuse(at, "a property of "+p.Name+"'s extension whose declaration this cannot find")
		return "", nil, true
	}
	subst := make(map[*types.TypeParam]types.Type, len(g.subst)+1)
	for k, v := range g.subst {
		subst[k] = v
	}
	subst[p.Self] = recv
	t := types.Substitute(pf.Type, subst)
	name := "$sVSCext_" + identifierSafe(p.Name) + "_" + identifierSafe(pf.Name) + "_g_Tv" + identifierSafe(recv.String())
	if g.specialized[name] {
		return name, t, true
	}
	if existing := g.m.Lookup(name); existing != nil && !existing.IsDeclaration() {
		return name, t, true
	}
	body := g.getterBody(decl.binding)
	if body == nil {
		g.refuse(at, "a property of "+p.Name+"'s extension with no getter")
		return "", nil, true
	}
	if g.specialized == nil {
		g.specialized = map[string]bool{}
	}
	g.specialized[name] = true

	restore := g.aside()
	endSpecialization := g.asSpecialization()
	defer endSpecialization()
	defer restore()
	if file := g.fileOf(decl.binding); file != nil {
		g.file = file
	}
	g.subst = subst
	g.emitGetterNamed(name, recv, pf.Name, t, body, false, sil.Private)
	return name, t, true
}
