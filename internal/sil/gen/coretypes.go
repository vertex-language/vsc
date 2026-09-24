package gen

import (
	"strings"

	"github.com/vertex-language/vsc/analyzer"
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/internal/sil"
	"github.com/vertex-language/vsc/mangle"
	"github.com/vertex-language/vsc/types"
)

// The core declares a few types of its own that are not generic --
// Character -- and gives them members in algorithms.swift. Like the rest
// of the core's source, those are lowered in the module that uses them,
// privately, so that no two modules' copies clash: a member is lowered
// once something in the module calls it, and the type's conformances once
// anything in the module names the type, since the runtime may be asked
// for them wherever a value of it goes as an Any.

// coreTypeMembers lowers what this module uses of the core's own types'
// members, and their witness tables, until nothing more is called for.
func (g *gen) coreTypeMembers() {
	core := g.info.CoreAlgorithms
	if core == nil {
		return
	}
	var exts []*ast.ExtensionDecl
	for _, st := range core.Stmts {
		decl, ok := st.(*ast.DeclStmt)
		if !ok {
			continue
		}
		ext, ok := decl.D.(*ast.ExtensionDecl)
		if !ok || ext.Body == nil {
			continue
		}
		if recv := g.info.Extensions[ext]; g.coreOwnType(recv) || g.coreBuiltinValue(recv) {
			exts = append(exts, ext)
		}
	}
	if len(exts) == 0 {
		return
	}
	tabled := map[types.Type]bool{}
	for more := true; more; {
		more = false
		for _, ext := range exts {
			recv := g.info.Extensions[ext]
			for _, mem := range ext.Body.Members {
				if g.coreTypeMember(mem, recv) {
					more = true
				}
			}
		}
		for _, ext := range exts {
			recv := g.info.Extensions[ext]
			if !g.coreOwnType(recv) || tabled[recv] || !g.usesType(recv) {
				continue
			}
			tabled[recv] = true
			more = true
			restore := g.apart()
			g.file, g.specializing = g.info.CoreAlgorithms.Unit, true
			for _, p := range g.conformancesOf(recv) {
				g.witnessTable(ext, recv, p)
			}
			restore()
		}
	}
}

// coreOwnType reports whether t is a type the core declares as a
// nominal type, rather than one of the built-in types, and not generic.
func (g *gen) coreOwnType(t types.Type) bool {
	if t == nil || !g.isCoreType(t) || extendedBuiltin(t) != nil || builtinParams(g.info, t) != nil {
		return false
	}
	switch t.Underlying().(type) {
	case *types.Struct, *types.Enum:
		return len(nominalTypeParams(t)) == 0
	}
	return false
}

// coreBuiltinValue reports whether t is a built-in type that is not
// generic -- String, Int -- whose core extensions may declare subscripts,
// which are lowered here as a core type's members are. (Their methods,
// properties and initializers have ways of their own; see emitCoreInit.)
func (g *gen) coreBuiltinValue(t types.Type) bool {
	return t != nil && extendedBuiltin(t) != nil && builtinParams(g.info, t) == nil && isBasicValue(t)
}

// usesType reports whether anything in the module so far names t: a
// function of its, called or defined.
func (g *gen) usesType(t types.Type) bool {
	// A value of it made into an Any names its metadata.
	if _, named := g.m.MetadataFor(typeNameOf(t)); named {
		return true
	}
	prefix, err := mangle.NominalType(mangle.Decl{Module: g.moduleOfType(t), Context: nominalChain(t), ModuleOf: g.moduleOfType})
	if err != nil {
		return false
	}
	for _, f := range g.m.Funcs() {
		if strings.HasPrefix(f.Name(), prefix) {
			return true
		}
	}
	return false
}

// coreTypeMember lowers one member of a core type's extension if the
// module calls it and it is not lowered yet, and reports whether it was.
func (g *gen) coreTypeMember(mem ast.Node, recv types.Type) bool {
	wanted := func(symbol string) bool {
		f := g.m.Lookup(symbol)
		return f != nil && f.IsDeclaration()
	}
	restore := g.apart()
	defer restore()
	g.file, g.specializing = g.info.CoreAlgorithms.Unit, true
	if sub, ok := mem.(*ast.SubscriptDecl); ok {
		return g.coreSubscript(sub, recv, wanted)
	}
	if !g.coreOwnType(recv) {
		return false
	}
	switch m := mem.(type) {
	case *ast.FuncDecl:
		sym, _ := g.info.Defs[m.Name].(*analyzer.FuncSymbol)
		if sym == nil {
			return false
		}
		name := g.methodSymbol(&analyzer.MethodRef{Recv: recv,
			Method: &types.Method{Name: sym.Name(), Sig: sym.Signature(), IsStatic: isStaticDecl(m.Mods)}})
		if !wanted(name) {
			return false
		}
		g.function(m, recv)
		return true
	case *ast.InitDecl:
		sig := g.initSignature(m, recv)
		if sig == nil || !wanted(g.initSymbol(recv, sig)) {
			return false
		}
		g.initializer(m, recv)
		return true
	case *ast.VarDecl:
		static := isStaticDecl(m.Mods)
		lowered := false
		for _, b := range m.Bindings {
			name, t := g.computedName(b), g.bindingType(b, recv)
			body := g.getterBody(b)
			if name == "" || t == nil || body == nil {
				continue
			}
			d := mangle.Decl{Module: g.memberModule(recv, nil), Context: memberChain(recv),
				Name: name, Signature: &types.Signature{Results: t}, ModuleOf: g.moduleOfType}
			symbol, err := mangle.Getter(d)
			if static {
				symbol, err = mangle.StaticGetter(d)
			}
			if err != nil || !wanted(symbol) {
				continue
			}
			g.emitGetterNamed(symbol, recv, name, t, body, static, sil.Private)
			lowered = true
		}
		return lowered
	}
	return false
}

// coreSubscript lowers the getter of a subscript a core extension
// declares, where the module calls it.
func (g *gen) coreSubscript(m *ast.SubscriptDecl, recv types.Type, wanted func(string) bool) bool {
	sub := g.info.SubscriptDecls[m]
	if sub == nil {
		return false
	}
	getter := m.Body
	if m.Accessors != nil {
		for _, a := range m.Accessors.Accessors {
			if a != nil && a.Keyword != nil && a.Body != nil && g.text(a.Keyword) == "get" {
				getter = a.Body
			}
		}
	}
	if getter == nil || !wanted(g.subscriptSymbol(m, recv, sub, false)) {
		return false
	}
	g.emitSubscriptAccessor(m, recv, sub, getter, nil, sil.Private)
	return true
}
