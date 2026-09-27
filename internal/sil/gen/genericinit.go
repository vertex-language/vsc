package gen

import (
	"strings"

	"github.com/vertex-language/vsc/analyzer"
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/internal/sil"
	"github.com/vertex-language/vsc/types"
)

// Initializers of generic classes are lowered by monomorphisation, as
// their methods are: for each instance one makes, with the class's
// parameters standing for the instance's arguments. The unspecialized
// initializers are not lowered: their parameters are no signature's.

// genericInitDecls is where every initializer of a generic type in the
// module was written.
func genericInitDecls(files []*ast.File, info *analyzer.Info) map[types.Type][]*ast.InitDecl {
	out := map[types.Type][]*ast.InitDecl{}
	add := func(t types.Type, body *ast.MemberBlock) {
		if t == nil || body == nil || len(nominalTypeParams(t)) == 0 {
			return
		}
		for _, mem := range body.Members {
			if d, ok := mem.(*ast.InitDecl); ok {
				out[t] = append(out[t], d)
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

// genericClassInit lowers a call to an initializer of a generic class for
// the instance it makes, having lowered that initializer for it. generic
// is false for a class that is not generic; a nil value with generic true
// means the call was refused.
func (g *gen) genericClassInit(e *ast.CallExpr, t types.Type, sig *types.Signature, args []*ast.CallArg) (*sil.Value, bool) {
	inst, ok := t.(*types.GenericInstance)
	if !ok {
		return nil, false
	}
	params := nominalTypeParams(inst.Base)
	if len(params) == 0 {
		return nil, false
	}
	if len(inst.Args) != len(params) {
		g.refuse(e, "an initializer of a generic class whose type arguments are not known")
		return nil, true
	}
	var decl *ast.InitDecl
	for _, d := range g.inits[inst.Base] {
		if g.sameInitParams(sig, d) {
			decl = d
			break
		}
	}
	if decl == nil {
		g.refuse(e, "an initializer of a generic class whose declaration this cannot find")
		return nil, true
	}
	subst := make(map[*types.TypeParam]types.Type, len(g.subst)+len(params))
	for k, v := range g.subst {
		subst[k] = v
	}
	for i, p := range params {
		subst[p] = inst.Args[i]
	}
	specialized, ok := types.Substitute(sig, subst).(*types.Signature)
	if !ok {
		g.refuse(e, "an initializer of a generic class whose signature this cannot substitute")
		return nil, true
	}
	named := *specialized
	named.Results = inst.Base
	alloc := g.initSymbol(inst.Base, &named)
	if alloc == "" || !strings.HasSuffix(alloc, "fC") {
		g.refuse(e, "an initializer of a generic class this compiler cannot name")
		return nil, true
	}
	// The instance's arguments before the allocating entry's suffix, so
	// the one that fills the instance in is still its name ending in c.
	var b strings.Builder
	b.WriteString(alloc[:len(alloc)-2])
	b.WriteString("Tv")
	for _, a := range inst.Args {
		b.WriteString(identifierSafe(a.String()))
	}
	b.WriteString("fC")
	alloc = b.String()
	initializing := alloc[:len(alloc)-1] + "c"

	out := *specialized
	out.Results = inst
	g.emitClassInitSpecialization(decl, inst, &out, alloc, initializing, subst)
	return g.applyInitNamed(e, inst, &out, args, alloc), true
}

// emitClassInitSpecialization lowers a generic class's initializer for one
// instance, once, reading it in the file it was written in.
func (g *gen) emitClassInitSpecialization(decl *ast.InitDecl, inst types.Type, sig *types.Signature,
	alloc, initializing string, subst map[*types.TypeParam]types.Type) {
	if g.specialized[alloc] {
		return
	}
	if existing := g.m.Lookup(alloc); existing != nil && !existing.IsDeclaration() {
		return
	}
	if g.specialized == nil {
		g.specialized = map[string]bool{}
	}
	g.specialized[alloc] = true

	prevFile, prevSubst := g.file, g.subst
	defer func() { g.file, g.subst = prevFile, prevSubst }()
	if f := g.fileOf(decl); f != nil {
		g.file = f
	}
	g.subst = subst
	// The instance's table: its layout, which is the class's with the
	// arguments in place, for the destroyer to find what it owns, and --
	// where the class has subclasses or a superclass, so that a call
	// dispatches -- its methods specialized for it.
	if gi, ok := inst.(*types.GenericInstance); ok {
		g.instanceTable(gi)
	} else if g.m.VTableNamed(inst.String()) == nil {
		g.m.VTable(inst.String()).Layout = inst
	}
	defer g.asSpecialization()()
	g.classInitBody(decl, inst, sig, initializing)
	g.classAllocator(inst, sig, alloc, initializing)
}

// instanceTable makes the table of an instance of a generic class, once:
// its layout, which is the class's with the arguments in place, for the
// destroyer to find what it owns, and -- where the class has subclasses or
// a superclass, so that a call dispatches -- its methods specialized for it.
func (g *gen) instanceTable(inst *types.GenericInstance) {
	if g.m.VTableNamed(inst.String()) != nil {
		return
	}
	t := g.m.VTable(inst.String())
	t.Layout = inst
	if cl, ok := inst.Underlying().(*types.Class); ok && g.poly[cl.Declared()] {
		for _, s := range g.slots(inst) {
			t.Entry(s.member, s.impl)
		}
	}
	// Its deinit, lowered for the instance: what its destroyer runs
	// before letting go of what the instance holds.
	if d := g.classDeinitDecl(inst.Base); d != nil {
		t.Deinit = deinitSymbol(g.module, inst)
		g.emitDeinitSpecialization(d, inst)
	}
}

// classDeinitDecl is the deinit a class declares, or nil.
func (g *gen) classDeinitDecl(base types.Type) *ast.DeinitDecl {
	for _, f := range g.files {
		for _, stmt := range f.Stmts {
			ds, ok := stmt.(*ast.DeclStmt)
			if !ok {
				continue
			}
			cd, ok := ds.D.(*ast.ClassDecl)
			if !ok || cd.Name == nil || cd.Body == nil {
				continue
			}
			if sym, ok := g.info.Defs[cd.Name].(*analyzer.TypeNameSymbol); !ok || sym.Type() != base {
				continue
			}
			for _, m := range cd.Body.Members {
				if d, ok := m.(*ast.DeinitDecl); ok {
					return d
				}
			}
			return nil
		}
	}
	return nil
}

// emitDeinitSpecialization lowers a generic class's deinit for one
// instance, reading it in the file it was written in.
func (g *gen) emitDeinitSpecialization(d *ast.DeinitDecl, inst *types.GenericInstance) {
	params := nominalTypeParams(inst.Base)
	if len(params) != len(inst.Args) {
		return
	}
	subst := make(map[*types.TypeParam]types.Type, len(params))
	for i, p := range params {
		subst[p] = inst.Args[i]
	}
	prevFile, prevSubst := g.file, g.subst
	defer func() { g.file, g.subst = prevFile, prevSubst }()
	if f := g.fileOf(d); f != nil {
		g.file = f
	}
	g.subst = subst
	defer g.asSpecialization()()
	g.deinitializer(d, inst)
}

// genericStructInit lowers a call to a declared initializer of a generic
// struct for the instance it makes, having lowered that initializer for
// it. generic is false for a struct that is not generic; a nil value with
// generic true means the call was refused.
func (g *gen) genericStructInit(e *ast.CallExpr, t types.Type, sig *types.Signature, args []*ast.CallArg) (*sil.Value, bool) {
	inst, ok := t.(*types.GenericInstance)
	if !ok {
		return nil, false
	}
	params := nominalTypeParams(inst.Base)
	if len(params) == 0 {
		return nil, false
	}
	if !isStructType(inst.Base) && !isEnumType(inst.Base) {
		return nil, false
	}
	if len(inst.Args) != len(params) {
		g.refuse(e, "an initializer of a generic struct whose type arguments are not known")
		return nil, true
	}
	var decl *ast.InitDecl
	for _, d := range g.inits[inst.Base] {
		if g.sameInitParams(sig, d) {
			decl = d
			break
		}
	}
	if decl == nil {
		g.refuse(e, "an initializer of a generic struct whose declaration this cannot find")
		return nil, true
	}
	subst := make(map[*types.TypeParam]types.Type, len(g.subst)+len(params))
	for k, v := range g.subst {
		subst[k] = v
	}
	for i, p := range params {
		subst[p] = inst.Args[i]
	}
	specialized, ok := types.Substitute(sig, subst).(*types.Signature)
	if !ok {
		g.refuse(e, "an initializer of a generic struct whose signature this cannot substitute")
		return nil, true
	}
	named := *specialized
	named.Results = inst.Base
	base := g.initSymbol(inst.Base, &named)
	if base == "" {
		g.refuse(e, "an initializer of a generic struct this compiler cannot name")
		return nil, true
	}
	name := specializedInitName(base, inst.Args)
	out := *specialized
	out.Results = inst
	g.emitStructInitSpecialization(decl, inst, &out, name, subst)
	return g.applyInitNamed(e, inst, &out, args, name), true
}

// specializedInitName is an initializer's symbol for one instance: its
// arguments before the entry's suffix, where there is one.
func specializedInitName(base string, args []types.Type) string {
	var b strings.Builder
	suffix := ""
	if strings.HasSuffix(base, "fC") {
		base, suffix = base[:len(base)-2], "fC"
	}
	b.WriteString(base)
	b.WriteString("Tv")
	for _, a := range args {
		b.WriteString(identifierSafe(a.String()))
	}
	b.WriteString(suffix)
	return b.String()
}

// emitStructInitSpecialization lowers a generic struct's initializer for
// one instance, once, reading it in the file it was written in.
func (g *gen) emitStructInitSpecialization(decl *ast.InitDecl, inst types.Type, sig *types.Signature,
	name string, subst map[*types.TypeParam]types.Type) {
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

	prevFile, prevSubst := g.file, g.subst
	defer func() { g.file, g.subst = prevFile, prevSubst }()
	if f := g.fileOf(decl); f != nil {
		g.file = f
	}
	g.subst = subst
	defer g.asSpecialization()()
	g.structInitBody(decl, inst, sig, name)
}

// protocolExtInit makes a t with an initializer a protocol's extension
// declares: its body lowered for t, with Self t, under a name that says
// which -- as a method of a protocol's extension is (see
// protocolExtensionMethod) -- and called as t's own initializer would be.
func (g *gen) protocolExtInit(e *ast.CallExpr, t types.Type, pi analyzer.ProtocolInit) *sil.Value {
	p := pi.Protocol
	decl := g.info.ProtocolInitDecls[pi.Sig]
	if decl == nil || p.Self == nil {
		g.refuse(e, "an initializer of "+p.Name+"'s extension whose declaration this cannot find")
		return nil
	}
	if !isStructType(t) && !isEnumType(t) {
		g.refuse(e, "an initializer of "+p.Name+"'s extension making something other than a struct or an enum")
		return nil
	}
	subst := make(map[*types.TypeParam]types.Type, len(g.subst)+1)
	for k, v := range g.subst {
		subst[k] = v
	}
	subst[p.Self] = t
	spec, ok := types.Substitute(pi.Sig, subst).(*types.Signature)
	if !ok {
		g.refuse(e, "an initializer of "+p.Name+"'s extension whose signature this cannot substitute")
		return nil
	}
	var name strings.Builder
	name.WriteString("$sVSCextinit_")
	name.WriteString(identifierSafe(p.Name))
	for _, prm := range pi.Sig.Params {
		name.WriteString("_")
		name.WriteString(identifierSafe(prm.Label))
	}
	name.WriteString("_Tv")
	name.WriteString(identifierSafe(t.String()))
	out := *spec
	out.Results = t
	g.emitStructInitSpecialization(decl, t, &out, name.String(), subst)
	var args []*ast.CallArg
	if e.Args != nil {
		args = e.Args.Args
	}
	return g.applyInitNamed(e, t, &out, args, name.String())
}
