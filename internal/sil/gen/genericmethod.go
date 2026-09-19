package gen

import (
	"strings"

	"github.com/vertex-language/vsc/analyzer"
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/internal/sil"
	"github.com/vertex-language/vsc/mangle"
	"github.com/vertex-language/vsc/types"
)

// Methods of generic types are lowered by monomorphisation, as generic
// functions are: for each instance a call is made on, with the type's
// parameters standing for the instance's arguments. The unspecialized
// method has no symbol -- its parameters are no signature's -- and is not
// lowered at all.

// genericMethodKey names a method of a generic type: the type as it was
// declared, and the method's name.
type genericMethodKey struct {
	typ  types.Type
	name string
}

// genericMethodDecls is where every method of a generic type in the
// module was written, in its type's body or in an extension of it: all
// of a name, since a name may be overloaded -- `first()` beside
// `first(where:)` -- and which one a call means is its signature's to
// say. See genericMethodDecl.
func genericMethodDecls(files []*ast.File, info *analyzer.Info) map[genericMethodKey][]*ast.FuncDecl {
	out := map[genericMethodKey][]*ast.FuncDecl{}
	add := func(t types.Type, body *ast.MemberBlock) {
		if t == nil || body == nil || (len(nominalTypeParams(t)) == 0 && builtinParams(info, t) == nil) {
			return
		}
		for _, mem := range body.Members {
			fd, ok := mem.(*ast.FuncDecl)
			if !ok || fd.Name == nil {
				continue
			}
			if fs, ok := info.Defs[fd.Name].(*analyzer.FuncSymbol); ok {
				key := genericMethodKey{typ: t, name: fs.Name()}
				out[key] = append(out[key], fd)
			}
		}
	}
	declared := func(name *ast.Ident) types.Type {
		if name == nil {
			return nil
		}
		if s, ok := info.Defs[name].(*analyzer.TypeNameSymbol); ok {
			return s.Type()
		}
		return nil
	}
	for _, f := range files {
		for _, stmt := range f.Stmts {
			decl, ok := stmt.(*ast.DeclStmt)
			if !ok {
				continue
			}
			switch d := decl.D.(type) {
			case *ast.StructDecl:
				add(declared(d.Name), d.Body)
			case *ast.ClassDecl:
				add(declared(d.Name), d.Body)
			case *ast.EnumDecl:
				add(declared(d.Name), d.Body)
			case *ast.ExtensionDecl:
				add(info.Extensions[d], d.Body)
			}
		}
	}
	return out
}

// builtinParams is the generic parameters of a built-in type's extensions,
// where t is one of Array, Dictionary, Set or Optional, and nil otherwise.
func builtinParams(info *analyzer.Info, t types.Type) []*types.TypeParam {
	if b := builtinOf(info, t); b != nil {
		return b.Params
	}
	return nil
}

// builtinOf is the members extensions gave t's built-in type, or nil.
func builtinOf(info *analyzer.Info, t types.Type) *analyzer.BuiltinMembers {
	if meta, ok := t.(*types.Metatype); ok {
		t = meta.Instance
	}
	if info == nil {
		return nil
	}
	return info.Builtins[analyzer.BuiltinKey(t)]
}

// nominalTypeParams is the type parameters a nominal type declares, or
// those of the type an instance is of.
func nominalTypeParams(t types.Type) []*types.TypeParam {
	if gi, ok := t.(*types.GenericInstance); ok {
		t = gi.Base
	}
	if t == nil {
		return nil
	}
	switch u := t.Underlying().(type) {
	case *types.Struct:
		return u.TypeParams
	case *types.Class:
		return u.TypeParams
	case *types.Enum:
		return u.TypeParams
	}
	return nil
}

// genericMethod is a call's method of a generic type, specialized for the
// instance it is called on: the reference to call and its symbol, having
// lowered it. generic is false for a method of a type that is not generic.
// An empty symbol with generic true means the call was refused.
func (g *gen) genericMethod(e *ast.CallExpr, ref *analyzer.MethodRef) (*analyzer.MethodRef, string, bool) {
	var inst *types.GenericInstance
	var recvT types.Type
	if mem, ok := e.Fun.(*ast.MemberExpr); ok {
		t := g.typeOf(mem.X)
		// A static method is called on the instance's type.
		if meta, isMeta := t.(*types.Metatype); isMeta {
			t = meta.Instance
		}
		inst, _ = t.(*types.GenericInstance)
		recvT = t
	}
	// A method an extension gives Array, Dictionary, Set or Optional is
	// the extension's, for the elements of the one it is called on --
	// which, inside a method being specialized, are the specialization's.
	if b := builtinOf(g.info, ref.Recv); b != nil && len(b.Params) > 0 {
		if recvT == nil || analyzer.BuiltinKey(recvT) != b.Key {
			recvT = g.recv
		}
		if mem, ok := e.Fun.(*ast.MemberExpr); ok {
			if _, onSelf := mem.X.(*ast.SelfExpr); onSelf && g.recv != nil {
				recvT = g.recv
			}
		}
		if len(g.subst) > 0 && recvT != nil {
			recvT = types.Substitute(recvT, g.subst)
		}
		return g.builtinMethod(e, ref, b, recvT)
	}
	// A method called on self, within a method specialized already.
	if inst == nil {
		inst, _ = g.recv.(*types.GenericInstance)
	}
	// The type the method was declared on: the instance's, when there is
	// one -- the reference may name the substituted type, which declares
	// no parameters -- and the reference's otherwise.
	base := ref.Recv
	if gi, ok := base.(*types.GenericInstance); ok {
		base = gi.Base
	}
	if inst != nil && len(nominalTypeParams(inst.Base)) > 0 {
		base = inst.Base
	}
	params := nominalTypeParams(base)
	if len(params) == 0 {
		return nil, "", false
	}
	if inst == nil || len(inst.Args) != len(params) {
		g.refuse(e, "a method of a generic type on something whose type arguments are not known")
		return nil, "", true
	}
	decl := g.genericMethodDecl(genericMethodKey{typ: base, name: ref.Method.Name}, ref.Method)
	if decl == nil {
		g.refuse(e, "a method of a generic type whose declaration this cannot find")
		return nil, "", true
	}
	subst := make(map[*types.TypeParam]types.Type, len(g.subst)+len(params))
	for k, v := range g.subst {
		subst[k] = v
	}
	for i, p := range params {
		subst[p] = inst.Args[i]
	}
	// A method with generic parameters of its own is specialized for what
	// this call inferred them to be, as well as for the instance.
	var own []types.Type
	if spec, ok := g.info.Specializations[e]; ok {
		for i, p := range spec.Params {
			if i >= len(spec.Args) || spec.Args[i] == nil {
				g.refuse(e, "a call whose type arguments could not be inferred")
				return nil, "", true
			}
			arg := spec.Args[i]
			if len(g.subst) > 0 {
				arg = types.Substitute(arg, g.subst)
			}
			subst[p] = arg
			own = append(own, arg)
		}
	}
	sig, ok := types.Substitute(ref.Method.Sig, subst).(*types.Signature)
	if !ok {
		g.refuse(e, "a method of a generic type whose signature this cannot substitute")
		return nil, "", true
	}
	mangled, err := mangle.Function(mangle.Decl{
		Module:    g.memberModule(base, ref.Method),
		Context:   memberChain(base),
		Extended:  extendedBuiltin(base),
		Name:      ref.Method.Name,
		Signature: sig,
		Static:    ref.Method.IsStatic,
		ModuleOf:  g.moduleOfType,
	})
	if err != nil {
		g.refuse(e, "a method of a generic type this compiler cannot name: "+err.Error())
		return nil, "", true
	}
	var b strings.Builder
	b.WriteString(mangled)
	b.WriteString("Tv")
	for _, a := range inst.Args {
		b.WriteString(identifierSafe(a.String()))
	}
	for _, a := range own {
		b.WriteString("_")
		b.WriteString(identifierSafe(a.String()))
	}
	name := b.String()
	spec := &analyzer.MethodRef{Recv: inst, Method: &types.Method{
		Name:       ref.Method.Name,
		Sig:        sig,
		IsStatic:   ref.Method.IsStatic,
		IsMutating: ref.Method.IsMutating,
	}}
	g.emitMethodSpecialization(decl, inst, name, subst)
	return spec, name, true
}

// emitMethodSpecialization lowers a generic type's method for one instance,
// once, reading it in the file it was written in.
func (g *gen) emitMethodSpecialization(decl *ast.FuncDecl, inst types.Type, name string, subst map[*types.TypeParam]types.Type) {
	if g.specialized[name] {
		return
	}
	if existing := g.m.Lookup(name); existing != nil && !existing.IsDeclaration() {
		return
	}
	if g.specialized == nil {
		g.specialized = map[string]bool{}
	}
	g.specialized[name] = true

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
	defer func() {
		g.fn, g.entry, g.blk = outer.fn, outer.entry, outer.blk
		g.scopes, g.locals = outer.scopes, outer.locals
		g.loops, g.pending = outer.loops, outer.pending
		g.recv, g.self, g.subst = outer.recv, outer.self, outer.subst
		g.throws, g.catches, g.tryBang, g.tryCall = outer.throws, outer.catches, outer.tryBang, outer.tryCall
		g.file = prevFile
	}()
	if f := g.fileOf(decl); f != nil {
		g.file = f
	}
	g.subst = subst
	g.functionNamed(decl, inst, name)
}

// genericMethodDecl is the declaration of the one method of a name a
// reference means: the one declared with the reference's signature, or,
// failing that, the one whose parameters are labelled as it is.
func (g *gen) genericMethodDecl(key genericMethodKey, m *types.Method) *ast.FuncDecl {
	decls := g.methods[key]
	if len(decls) == 0 {
		return nil
	}
	if len(decls) == 1 {
		return decls[0]
	}
	for _, fd := range decls {
		if fs, ok := g.info.Defs[fd.Name].(*analyzer.FuncSymbol); ok && fs.Signature() == m.Sig {
			return fd
		}
	}
	for _, fd := range decls {
		fs, ok := g.info.Defs[fd.Name].(*analyzer.FuncSymbol)
		if !ok || m.Sig == nil || len(fs.Signature().Params) != len(m.Sig.Params) {
			continue
		}
		same := true
		for i, p := range fs.Signature().Params {
			if p.Label != m.Sig.Params[i].Label {
				same = false
			}
		}
		if same {
			return fd
		}
	}
	return decls[0]
}

// builtinMethod is genericMethod for a method an extension gives a generic
// built-in type, called on recv: [Int] for a method of Array.
func (g *gen) builtinMethod(e *ast.CallExpr, ref *analyzer.MethodRef, b *analyzer.BuiltinMembers, recv types.Type) (*analyzer.MethodRef, string, bool) {
	args := b.Args(recv)
	if recv == nil || len(args) != len(b.Params) {
		g.refuse(e, "a method of "+b.Key+" on something whose element types are not known")
		return nil, "", true
	}
	decl := g.genericMethodDecl(genericMethodKey{typ: b.Type, name: ref.Method.Name}, ref.Method)
	if decl == nil {
		g.refuse(e, "a method of "+b.Key+" whose declaration this cannot find")
		return nil, "", true
	}
	subst := make(map[*types.TypeParam]types.Type, len(g.subst)+len(b.Params))
	for k, v := range g.subst {
		subst[k] = v
	}
	for i, p := range b.Params {
		subst[p] = args[i]
	}
	var own []types.Type
	if spec, ok := g.info.Specializations[e]; ok {
		for i, p := range spec.Params {
			if i >= len(spec.Args) || spec.Args[i] == nil {
				g.refuse(e, "a call whose type arguments could not be inferred")
				return nil, "", true
			}
			arg := spec.Args[i]
			if len(g.subst) > 0 {
				arg = types.Substitute(arg, g.subst)
			}
			subst[p] = arg
			own = append(own, arg)
		}
	}
	sig, ok := types.Substitute(ref.Method.Sig, subst).(*types.Signature)
	if !ok {
		g.refuse(e, "a method of "+b.Key+" whose signature this cannot substitute")
		return nil, "", true
	}
	mangled, err := mangle.Function(mangle.Decl{
		Module:    g.memberModule(b.Type, ref.Method),
		Extended:  b.Type,
		Name:      ref.Method.Name,
		Signature: sig,
		Static:    ref.Method.IsStatic,
		ModuleOf:  g.moduleOfType,
	})
	if err != nil {
		g.refuse(e, "a method of "+b.Key+" this compiler cannot name: "+err.Error())
		return nil, "", true
	}
	var name strings.Builder
	name.WriteString(mangled)
	name.WriteString("Tv")
	for _, a := range args {
		name.WriteString(identifierSafe(a.String()))
	}
	for _, a := range own {
		name.WriteString("_")
		name.WriteString(identifierSafe(a.String()))
	}
	concrete := types.Substitute(b.Type, subst)
	spec := &analyzer.MethodRef{Recv: concrete, Method: &types.Method{
		Name:       ref.Method.Name,
		Sig:        sig,
		IsStatic:   ref.Method.IsStatic,
		IsMutating: ref.Method.IsMutating,
	}}
	g.emitMethodSpecialization(decl, concrete, name.String(), subst)
	return spec, name.String(), true
}
