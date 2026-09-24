package gen

import (
	"strings"

	"github.com/vertex-language/vsc/analyzer"
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/derive"
	"github.com/vertex-language/vsc/internal/sil"
	"github.com/vertex-language/vsc/mangle"
	"github.com/vertex-language/vsc/types"
)

// Existential and witness table lowering.
//
// Swift existentials occupy 5 words: 3 buffer words, 1 metadata pointer,
// and 1 witness table pointer. Each witness table entry thunks indirect
// receiver addresses into concrete parameter values.

// existentialWords is how many words an existential occupies.
const existentialWords = 5

// existentialBufferWords is how many of them the value itself may
// use before it would have to be boxed.
const existentialBufferWords = 3

// witnessTables emits one table per conformance in the module.
func (g *gen) witnessTables(files []*ast.File) {
	for _, f := range files {
		for _, stmt := range f.Stmts {
			decl, ok := stmt.(*ast.DeclStmt)
			if !ok {
				continue
			}
			var name *ast.Ident
			switch d := decl.D.(type) {
			case *ast.ProtocolDecl:
				// A protocol this module declares has its descriptor
				// here, which conformances to it point at.
				if sym, ok := g.info.Defs[d.Name].(*analyzer.TypeNameSymbol); ok && d.Name != nil {
					if p, ok := sym.Type().(*types.Protocol); ok {
						if s, err := mangle.ProtocolDescriptor(g.module, p.Name); err == nil {
							g.m.ProtocolDescriptor(s)
						}
					}
				}
				continue
			case *ast.StructDecl:
				name = d.Name
			case *ast.ClassDecl:
				name = d.Name
			case *ast.EnumDecl:
				name = d.Name
			default:
				continue
			}
			if name == nil {
				continue
			}
			sym, _ := g.info.Defs[name].(*analyzer.TypeNameSymbol)
			if sym == nil {
				continue
			}
			// A generic type's conformances are per instance, and none is
			// lowered yet: its methods are, where they are called.
			if len(nominalTypeParams(sym.Type())) > 0 {
				continue
			}
			for _, p := range g.conformancesOf(sym.Type()) {
				g.witnessTable(name, sym.Type(), p)
			}
		}
	}
}

// witnessTable emits the table for one conformance, and the thunks
// its rows name.
func (g *gen) witnessTable(at ast.Node, concrete types.Type, p *types.Protocol) {
	if p == nil || concrete == nil {
		return
	}
	// Avoid emitting duplicate witness tables across compilation units.
	if hasWitnessTable(g.m, typeNameOf(concrete), p.Name) {
		return
	}
	if _, ok := g.layoutOrder(at, p); !ok {
		return
	}
	// Emit tables for inherited protocols first.
	for _, up := range p.Inherited {
		if up != nil {
			g.witnessTable(at, concrete, up)
		}
	}
	// A conformance is found at run time by the type's descriptor, so a
	// type that conforms to anything has its metadata emitted, whether or
	// not this module ever asks for it: another module's Set may.
	if !g.importedType(concrete) && extendedBuiltin(concrete) == nil {
		if _, isGeneric := concrete.(*types.GenericInstance); !isGeneric && len(typeParamsOfType(concrete)) == 0 {
			g.needMetadata(at, concrete)
		}
	}
	table := g.m.WitnessTable(typeNameOf(concrete), p.Name, g.module, sil.Hidden)
	if sym, descr, ok := g.conformanceSymbols(at, concrete, p); ok {
		table.Symbol, table.Descriptor = sym, descr
		// The protocol's descriptor, wherever it is defined: core's in the
		// runtime, another module's in that module, this module's here
		// (see protocolDescriptors). A conformance names it, and the
		// runtime finds a conformance by it.
		if s, err := mangle.ProtocolDescriptor(g.moduleOfType(p), p.Name); err == nil {
			table.ProtocolSymbol = s
		}
	}
	for _, up := range p.Inherited {
		if up != nil {
			table.Entry(baseRow(p, up), up.Name)
		}
	}
	for _, r := range p.Requirements {
		if r != nil && r.Sig == nil && r.Type != nil {
			getterThunk := g.getterWitnessThunk
			if r.IsStatic {
				getterThunk = g.staticGetterWitnessThunk
			}
			thunk, ok := getterThunk(concrete, p, r)
			if !ok {
				g.errorAt(at, "'"+typeNameOf(concrete)+"' does not provide '"+
					r.Name+"', which '"+p.Name+"' requires")
				continue
			}
			table.Entry(p.Name+"."+r.Name, thunk)
			continue
		}
		if r == nil || r.Sig == nil {
			continue
		}
		if requiresSelfOperands(p, r) {
			thunk, ok := g.staticWitnessThunk(concrete, p, r)
			if !ok {
				g.errorAt(at, "'"+typeNameOf(concrete)+"' does not provide '"+
					r.Name+"', which '"+p.Name+"' requires")
				continue
			}
			table.Entry(p.Name+"."+r.Name, thunk)
			continue
		}
		found, m := g.methodMatching(concrete, r)
		if m == nil && r.Name == "makeIterator" && p.Name == "Sequence" && g.info.CoreTypes[p] {
			// A Sequence that is its own iterator: makeIterator() is a
			// copy of self, as Swift's default makes it.
			if _, next := g.methodOn(concrete, "next"); next != nil {
				table.Entry(p.Name+"."+r.Name, g.selfIteratorThunk(concrete, p))
				continue
			}
		}
		if m == nil {
			// The default a protocol extension gives, specialized for
			// the conformer.
			if thunk, ok := g.extensionWitness(at, concrete, p, r); ok {
				table.Entry(p.Name+"."+r.Name, thunk)
				continue
			}
			g.errorAt(at, "'"+typeNameOf(concrete)+"' does not provide '"+
				r.Name+"', which '"+p.Name+"' requires")
			continue
		}
		thunk := g.witnessThunk(concrete, found, p, m, r.Sig)
		if thunk == "" {
			continue
		}
		table.Entry(p.Name+"."+r.Name, thunk)
	}
}

// witnessThunk emits the function a table row names: the receiver
// arrives as an address, and what the method wants is the value.
func (g *gen) witnessThunk(concrete, found types.Type, p *types.Protocol, m *types.Method, req *types.Signature) string {
	return g.witnessThunkCalling(concrete, found, p, m, req, "")
}

// witnessThunkCalling is witnessThunk for an implementation named symbol --
// a protocol extension's default, specialized for the conformer -- or,
// where symbol is "", the method's own.
func (g *gen) witnessThunkCalling(concrete, found types.Type, p *types.Protocol, m *types.Method, req *types.Signature, symbol string) string {
	if req == nil {
		req = m.Sig
	}
	name := witnessThunkSymbol(concrete, p, m)
	if existing := g.m.Lookup(name); existing != nil && !existing.IsDeclaration() {
		return name
	}
	if m.IsStatic {
		return g.staticMethodWitnessThunk(name, concrete, found, m)
	}
	f := g.m.Func(name).SetSourceName(m.Name).SetLinkage(sil.Private).SetAttr("ossa")

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
	}{g.fn, g.entry, g.blk, g.scopes, g.locals, g.loops, g.pending, g.recv, g.self}
	defer func() {
		g.fn, g.entry, g.blk = outer.fn, outer.entry, outer.blk
		g.scopes, g.locals = outer.scopes, outer.locals
		g.loops, g.pending = outer.loops, outer.pending
		g.recv, g.self = outer.recv, outer.self
	}()

	g.fn, g.entry = f, false
	f.Type().Params = nil
	g.locals = map[analyzer.Symbol]*local{}
	g.scopes, g.loops, g.pending = nil, nil, ""
	g.recv, g.self = nil, nil
	g.push()
	g.blk = f.Entry()

	// The declared parameters, then the receiver -- as an address,
	// which is the whole difference from the method itself.
	ct := lowerType(concrete)
	var args []*sil.Value
	for i, param := range m.Sig.Params {
		t := lowerType(param.BodyType())
		conv := paramConvention(param, t)
		// An inout parameter is the caller's storage, as the method
		// declares it (declareMethod): the thunk passes the address on.
		if byAddress(conv) {
			t = t.Address()
		}
		v := f.Param(t, conv)
		args = append(args, v)
		if param.Name != "" {
			g.blk.DebugValue(v, param.Name, "let", "argno "+itoa(i+1))
		}
	}
	// A mutating requirement is handed the conformer's storage to change,
	// and the method is given that same storage rather than a copy.
	selfConv := sil.ParamInGuaranteed
	if m.IsMutating {
		selfConv = sil.ParamInout
	}
	selfAddr := f.Param(ct.Address(), selfConv)
	f.Type().Convention = sil.ConvWitness
	if m.Sig.Results != nil && !isVoid(m.Sig.Results) {
		rt := lowerType(m.Sig.Results)
		f.SetResult(rt, resultConvention(rt))
	}
	// The thunk is what a caller through the table calls, so it has the
	// requirement's effects: async where the call suspends, and throwing
	// where what the method throws has to reach that caller.
	f.Type().Async = req.Async
	if req.Throws {
		f.SetThrows(sil.Object(sil.BuiltinNativeObj))
	}

	self := selfAddr
	if !m.IsMutating {
		self = g.blk.Load(selfAddr, loadQualifier(ct))
	}
	if symbol == "" {
		symbol = g.methodSymbol(&analyzer.MethodRef{Recv: found, Method: m})
	}
	callee := g.m.Func(symbol).SetSourceName(m.Name)
	if g.needsType(callee) {
		g.declareMethod(callee, &analyzer.MethodRef{Recv: found, Method: m})
	}
	ref := g.blk.FunctionRef(callee)
	args = append(args, self)

	// The copy of the receiver loaded for the call is the thunk's to end.
	endSelf := func() {
		if !m.IsMutating && !ct.Trivial() {
			g.blk.DestroyValue(self)
		}
	}
	void := m.Sig.Results == nil || isVoid(m.Sig.Results)
	// The method's own effects decide how it is called: one that cannot
	// throw meets a requirement that may, and is applied plainly.
	if m.Sig.Throws && req.Throws {
		// try_apply: the value comes back on one edge, the error box on
		// the other, and the thunk throws that box on to its caller.
		normal, failed := f.Block(), f.Block()
		var out *sil.Value
		if !void {
			out = normal.Arg(lowerType(m.Sig.Results), sil.Owned)
		}
		box := failed.Arg(errorBoxType(), sil.Owned)
		g.blk.TryApply(ref, normal, failed, args...)
		g.blk = failed
		endSelf()
		g.blk.Throw(box)
		g.blk = normal
		endSelf()
		if void {
			g.blk.Return(g.void())
		} else {
			g.blk.Return(out)
		}
		return name
	}
	if void {
		g.blk.Apply(ref, lowerType(m.Sig.Results), args...)
		endSelf()
		g.blk.Return(g.void())
		return name
	}
	out := g.blk.Apply(ref, lowerType(m.Sig.Results), args...)
	endSelf()
	g.blk.Return(out)
	return name
}

// extensionWitness is the row for requirement r of p where the conformer
// writes no implementation and an extension of p -- or of a protocol p
// inherits -- gives the default: that method, specialized for concrete.
func (g *gen) extensionWitness(at ast.Node, concrete types.Type, p *types.Protocol, r *types.Requirement) (string, bool) {
	if r.Sig == nil || r.IsStatic {
		return "", false
	}
	want, _ := types.Substitute(r.Sig, map[*types.TypeParam]types.Type{p.Self: concrete}).(*types.Signature)
	for _, em := range p.ExtensionMethods(r.Name, false) {
		owner := p
		if q, found := p.ExtensionMethod(em.Name, false); found == em && q != nil {
			owner = q
		}
		got, _ := types.Substitute(em.Sig, map[*types.TypeParam]types.Type{owner.Self: concrete}).(*types.Signature)
		if got == nil || want == nil || !types.Identical(got, want) {
			continue
		}
		spec, symbol, ok := g.protocolExtensionMethod(nil, &analyzer.MethodRef{Recv: owner, Method: em}, owner, concrete)
		if !ok || spec == nil {
			return "", false
		}
		thunk := g.witnessThunkCalling(concrete, concrete, p, spec.Method, r.Sig, symbol)
		return thunk, thunk != ""
	}
	return "", false
}

// selfIteratorThunk is the makeIterator() row of a Sequence that is its
// own iterator: the receiver, copied.
func (g *gen) selfIteratorThunk(concrete types.Type, p *types.Protocol) string {
	m := &types.Method{Name: "makeIterator", Sig: &types.Signature{Results: concrete}}
	name := witnessThunkSymbol(concrete, p, m)
	if existing := g.m.Lookup(name); existing != nil && !existing.IsDeclaration() {
		return name
	}
	f := g.m.Func(name).SetSourceName(m.Name).SetLinkage(sil.Private).SetAttr("ossa")
	outerFn, outerEntry, outerBlk := g.fn, g.entry, g.blk
	defer func() { g.fn, g.entry, g.blk = outerFn, outerEntry, outerBlk }()
	g.fn, g.entry = f, false
	f.Type().Params = nil
	g.blk = f.Entry()
	ct := lowerType(concrete)
	selfAddr := f.Param(ct.Address(), sil.ParamInGuaranteed)
	f.Type().Convention = sil.ConvWitness
	f.SetResult(ct, resultConvention(ct))
	g.blk.Return(g.blk.Load(selfAddr, loadQualifier(ct)))
	return name
}

// staticMethodWitnessThunk emits the row for a static requirement that is
// not an operator: the declared parameters, then the conformer's type in
// the self register, and the conformer's own static method answers.
func (g *gen) staticMethodWitnessThunk(name string, concrete, found types.Type, m *types.Method) string {
	f := g.m.Func(name).SetSourceName(m.Name).SetLinkage(sil.Private).SetAttr("ossa")
	outerFn, outerEntry, outerBlk := g.fn, g.entry, g.blk
	defer func() { g.fn, g.entry, g.blk = outerFn, outerEntry, outerBlk }()
	g.fn, g.entry = f, false
	f.Type().Params = nil
	g.blk = f.Entry()

	var args []*sil.Value
	for _, param := range m.Sig.Params {
		t := lowerType(param.Type)
		args = append(args, f.Param(t, paramConvention(param, t)))
	}
	f.Param(sil.ThickMetatype(concrete), sil.ParamUnowned)
	f.Type().Convention = sil.ConvWitness
	rt := lowerType(m.Sig.Results)
	if m.Sig.Results != nil && !isVoid(m.Sig.Results) {
		f.SetResult(rt, resultConvention(rt))
	}

	ref := &analyzer.MethodRef{Recv: found, Method: m}
	callee := g.m.Func(g.staticSymbol(ref, concrete)).SetSourceName(m.Name)
	if g.needsType(callee) {
		g.declareSignature(callee, m.Sig)
	}
	out := g.blk.Apply(g.blk.FunctionRef(callee), rt, args...)
	if m.Sig.Results == nil || isVoid(m.Sig.Results) {
		g.blk.Return(g.void())
		return name
	}
	g.blk.Return(out)
	return name
}

// existentialCall calls a requirement through the table the value
// carries.
func (g *gen) existentialCall(e *ast.CallExpr, ref *analyzer.MethodRef, mem *ast.MemberExpr, ex *types.Existential) *sil.Value {
	var args []ast.Expr
	if e.Args != nil {
		for _, a := range e.Args.Args {
			args = append(args, a.X)
		}
	}
	return g.witnessApply(e, mem.X, ex, ref.Method, args)
}

// existentialProperty reads a property a protocol of the existential
// requires, through the getter row of the conformance's table, and
// reports whether the member is one.
func (g *gen) existentialProperty(e *ast.MemberExpr, ex *types.Existential) (*sil.Value, bool) {
	name := g.text(e.Name)
	for _, p := range ex.Protocols {
		for _, r := range allRequirements(p) {
			if r.Name == name && r.Sig == nil && r.Type != nil {
				m := &types.Method{Name: name, Sig: &types.Signature{Results: r.Type}}
				return g.witnessApply(e, e.X, ex, m, nil), true
			}
		}
	}
	return nil, false
}

// allRequirements is a protocol's requirements and those of every
// protocol it inherits.
func allRequirements(p *types.Protocol) []*types.Requirement {
	if p == nil {
		return nil
	}
	out := append([]*types.Requirement(nil), p.Requirements...)
	for _, up := range p.Inherited {
		out = append(out, allRequirements(up)...)
	}
	return out
}

// witnessApply calls the witness for a requirement on the existential x
// evaluates to: x's value opened in place, the arguments, the row found by
// the requirement's name.
func (g *gen) witnessApply(at ast.Node, x ast.Expr, ex *types.Existential, m *types.Method, argExprs []ast.Expr) *sil.Value {
	p, path, ok := protocolProviding(ex, m.Name)
	if !ok {
		g.refuse(at, "a use of something no protocol of this existential promises")
		return nil
	}
	// Ensure requirement order is established before indexing the witness table.
	if _, ok := g.layoutOrder(at, p); !ok {
		return nil
	}
	addr := g.existentialPlace(x)
	if addr == nil {
		return nil
	}

	var args []*sil.Value
	want := existentialParams(m.Sig)
	for i, a := range argExprs {
		// As a direct call's arguments are: a temporary is made and ended
		// with the statement, a variable is borrowed where it is. An owned
		// copy passed to a borrowed parameter had no end, which a call
		// that may fail -- two ways out -- made plain.
		v := g.expr(a)
		if v == nil {
			return nil
		}
		args = append(args, g.boxArg(a, v, want, i))
	}

	method := g.blk.WitnessMethodOn(addr, witnessMember(path, m.Name), witnessType(m, ex))
	opened := g.blk.OpenExistentialAddr(addr, lowerType(ex).Address())
	args = append(args, opened)

	// A throwing requirement is try_applied, as a call to the method
	// itself would be: its error is caught, turned optional or raised.
	if call, isCall := at.(*ast.CallExpr); isCall && m.Sig.Throws {
		optional, trap := g.tryOn(call)
		g.tryBang = trap
		return g.tryApply(call, method, args, m.Sig.Results, optional, false)
	}
	v := g.blk.Apply(method, lowerType(m.Sig.Results), args...)
	g.destroyLater(v)
	return v
}

// witnessType is the function type a witness has: the declared
// parameters, then the receiver as an address.
func witnessType(m *types.Method, ex *types.Existential) sil.Type {
	ft := &sil.FuncType{Convention: sil.ConvWitness}
	for _, p := range m.Sig.Params {
		t := lowerType(p.BodyType())
		conv := paramConvention(p, t)
		if byAddress(conv) {
			t = t.Address()
		}
		ft.Params = append(ft.Params, sil.Param{Type: t, Convention: conv})
	}
	// The receiver as the thunk takes it: the conformer's storage to change
	// for a mutating requirement, borrowed otherwise.
	selfConv := sil.ParamInGuaranteed
	if m.IsMutating {
		selfConv = sil.ParamInout
	}
	ft.Params = append(ft.Params, sil.Param{
		Type:       lowerType(ex).Address(),
		Convention: selfConv,
	})
	if m.Sig.Results != nil && !isVoid(m.Sig.Results) {
		t := lowerType(m.Sig.Results)
		ft.Results = append(ft.Results, sil.Result{Type: t, Convention: resultConvention(t)})
	}
	ft.Async = m.Sig.Async
	if m.Sig.Throws {
		ft.ErrorType = sil.Object(sil.BuiltinNativeObj)
	}
	return sil.Object(ft)
}

// protocolProviding is the protocol of an existential that promises a
// name, and the chain of tables that reaches it.
func protocolProviding(ex *types.Existential, name string) (*types.Protocol, []*types.Protocol, bool) {
	for _, p := range ex.Protocols {
		if p == nil {
			continue
		}
		if path, ok := witnessPath(p, name); ok {
			return p, path, true
		}
	}
	return nil, nil, false
}

// layoutOrder determines the layout order of protocol requirements in the witness table.
func (g *gen) layoutOrder(at ast.Node, p *types.Protocol) ([]string, bool) {
	if p == nil {
		return nil, false
	}
	names := make([]string, 0, len(p.Inherited)+len(p.Requirements))
	for _, up := range p.Inherited {
		if up == nil {
			continue
		}
		if _, ok := g.layoutOrder(at, up); !ok {
			return nil, false
		}
		names = append(names, baseRow(p, up))
	}
	for _, r := range p.Requirements {
		if r == nil || (r.Sig == nil && r.Type == nil) {
			continue
		}
		// A property requirement's row is its getter.
		names = append(names, p.Name+"."+r.Name)
	}
	g.m.Requirements(p.Name, names)
	return names, true
}

// baseRow returns the entry key for an inherited protocol witness table.
func baseRow(p, up *types.Protocol) string { return p.Name + ":" + up.Name }

// witnessPath resolves the protocol inheritance path to the requirement.
func witnessPath(p *types.Protocol, name string) ([]*types.Protocol, bool) {
	if p == nil {
		return nil, false
	}
	for _, r := range p.Requirements {
		if r != nil && r.Name == name {
			return []*types.Protocol{p}, true
		}
	}
	for _, up := range p.Inherited {
		if rest, ok := witnessPath(up, name); ok {
			return append([]*types.Protocol{p}, rest...), true
		}
	}
	return nil, false
}

// witnessMember spells one requirement as the walk that reaches it:
// the protocols in order, then the name.
func witnessMember(path []*types.Protocol, name string) string {
	out := ""
	for i, p := range path {
		if i > 0 {
			out += ":"
		}
		out += p.Name
	}
	return out + "." + name
}

// conformancesOf is the protocols a nominal type declares it satisfies.
func (g *gen) conformancesOf(t types.Type) []*types.Protocol {
	if t == nil {
		return nil
	}
	if b := builtinOf(g.info, t); b != nil {
		return b.Conformances
	}
	switch n := t.Underlying().(type) {
	case *types.Struct:
		return n.Conformances
	case *types.Class:
		return n.Conformances
	case *types.Enum:
		return n.Conformances
	}
	return nil
}

func typeNameOf(t types.Type) string {
	switch n := t.Underlying().(type) {
	case *types.Struct:
		return n.Name
	case *types.Class:
		return n.Name
	case *types.Enum:
		return n.Name
	}
	return t.String()
}

// witnessTableSymbol names a conformance's table.
func witnessTableSymbol(concrete types.Type, p *types.Protocol) string {
	return "$sWT" + identifierSafe(typeNameOf(concrete)) + "_" + identifierSafe(p.Name)
}

// witnessThunkSymbol names one row's thunk. Swift ends such a symbol
// in TW, for "protocol witness"; this keeps the marker and builds the
// rest from the names, since nothing outside the module resolves it.
func witnessThunkSymbol(concrete types.Type, p *types.Protocol, m *types.Method) string {
	d := mangle.Decl{
		Module:    "witness",
		Name:      m.Name,
		Signature: m.Sig,
	}
	name, err := mangle.Function(d)
	if err != nil {
		name = "$sW" + identifierSafe(m.Name)
	}
	return name + "TW" + identifierSafe(typeNameOf(concrete)) + "_" + identifierSafe(p.Name)
}

// existentialOf returns the underlying Existential if t represents an existential or protocol type.
func existentialOf(t types.Type) (*types.Existential, bool) {
	switch n := t.(type) {
	case *types.Existential:
		return n, true
	case *types.Protocol:
		return &types.Existential{Protocols: []*types.Protocol{n}}, true
	}
	return nil, false
}

// existentialFor boxes a concrete value into an existential if the destination type requires it.
func (g *gen) existentialFor(at ast.Node, v *sil.Value, from, to types.Type) *sil.Value {
	ex, ok := existentialOf(to)
	if !ok || v == nil {
		return v
	}
	if _, already := existentialOf(from); already {
		return g.ownedExistential(v)
	}
	// Protocols with associated types cannot be represented in non-generic existentials here.
	for _, p := range ex.Protocols {
		if a, ok := associatedIn(p); ok {
			g.refuse(at, "a value in 'any "+p.Name+"': "+p.Name+" leaves '"+a+
				"' to the conforming type, and an existential here carries a witness "+
				"table but not the metadata that would say what it is")
			return v
		}
	}
	if len(ex.Protocols) > 0 && g.conformancesOf(from) == nil && from != nil && !conformsToAll(from, ex) {
		// Unconformed type; let checker handle error.
		return v
	}
	slot := g.blk.AllocStack(lowerType(ex))
	if !g.initExistential(at, slot, v, from, ex) {
		return v
	}
	return slot
}

// initExistential initializes an allocated existential buffer with a concrete value and metadata.
func (g *gen) initExistential(at ast.Node, slot, v *sil.Value, from types.Type, ex *types.Existential) bool {
	// A value wider than the buffer's three words is boxed: lowering
	// makes the box, and opening the existential finds the value in it.
	// One wider than four words is also passed to its witnesses by
	// address, which a witness thunk does not do yet.
	if size := types.Sizeof(from, types.DefaultTarget64); size > 32 && len(ex.Protocols) > 0 {
		g.refuse(at, "a value of "+size64(size)+" bytes in an existential of a protocol: "+
			"it is boxed, and a witness called on a boxed value wider than four words is not lowered")
		return false
	}
	// Validate that non-trivial payload types have appropriate value witness support.
	// A class instance is one reference, whose witnesses count it.
	classInAny := isClass(from)
	if t := lowerType(from); !t.Trivial() && g.writesWitnesses(from) && !ownsOnlyReferences(from) &&
		!enumOwnsOnlyReferences(from) && !classInAny {
		g.refuse(at, "a "+typeNameOf(from)+" in an existential: what is inside one is "+
			"copied and destroyed through a value witness table, and the one this "+
			"compiler writes for a type declared here copies bytes and nothing else")
		return false
	}
	// Look up type metadata.
	meta, ok := g.stdlibMetadata(at, from)
	if !ok {
		return false
	}
	for _, p := range ex.Protocols {
		g.witnessTable(at, from, p)
	}
	buf := g.blk.InitExistentialAddr(slot, lowerType(from), meta)
	// Store consumed value into the existential buffer.
	g.blk.Store(g.consume(v), buf, storeQualifier(lowerType(from)))
	return true
}

// boxArg boxes an argument into an optional or existential if expected by parameter type.
func (g *gen) boxArg(a ast.Expr, v *sil.Value, want []types.Type, i int) *sil.Value {
	if to, ok := g.info.CStrings[a]; ok {
		return g.cStringArg(v, to)
	}
	if i >= len(want) {
		return v
	}
	from := g.typeOf(a)
	// An optional wraps for the same reason an existential boxes: the
	// parameter is a different type from the argument, and something
	// has to make one into the other. See optional.go.
	if o, ok := optionalOf(want[i]); ok && v.Ownership() != sil.None {
		if _, already := optionalOf(from); !already {
			if _, isEx := existentialOf(o.Wrapped); !isEx {
				// The optional takes the argument over: a borrowed one is
				// copied, and an owned one's pending destroy becomes the
				// optional's, so it is destroyed once whatever the parameter's
				// convention.
				some := g.optionalFor(a, g.consume(v), from, want[i])
				g.destroyLater(some)
				return some
			}
		}
	}
	v = g.optionalFor(a, v, from, want[i])
	out := g.existentialFor(a, v, from, want[i])
	// The call borrows an existential argument (@in_guaranteed), so the
	// temporary made for it is the caller's, and ends after the call --
	// as swiftc's SILGen destroys it.
	if _, isEx := existentialOf(want[i]); isEx && out != nil && out.Type().IsAddress() && !g.storage[out] {
		g.destroyAddrLater(out)
	}
	return out
}

// existentialParams is the parameter types a call's arguments are
// going into, so that each can be boxed if the parameter wants one.
func existentialParams(sig *types.Signature) []types.Type {
	if sig == nil {
		return nil
	}
	out := make([]types.Type, len(sig.Params))
	for i, p := range sig.Params {
		out[i] = p.Type
	}
	return out
}

// hasWitnessTable reports whether the module already describes this
// conformance.
func hasWitnessTable(m *sil.Module, typ, proto string) bool {
	for _, t := range m.WitnessTables() {
		if t != nil && t.Type == typ && t.Protocol == proto {
			return true
		}
	}
	return false
}

// size64 spells a byte count.
func size64(n int64) string { return itoa(int(n)) }

// associatedIn is the name of an associated type a protocol or one of
// its inherited protocols declares, and whether there was one.
func associatedIn(p *types.Protocol) (string, bool) {
	if p == nil {
		return "", false
	}
	for _, a := range p.Associated {
		if a != nil {
			return a.Name, true
		}
	}
	for _, up := range p.Inherited {
		if name, ok := associatedIn(up); ok {
			return name, true
		}
	}
	return "", false
}

// needMetadata emits runtime type metadata records for nominal types.
func (g *gen) needMetadata(at ast.Node, t types.Type) bool {
	if t == nil {
		return false
	}
	// A core type is named as the standard library's so that every module
	// spells it the same way, but no standard library ships its metadata:
	// this compiler's runtime provides the type, so each module that needs
	// the record emits its own. They do not collide -- a core type is not
	// in publicTypes, so its accessor stays internal to the module.
	if g.importedType(t) {
		// Declared elsewhere, so its module emitted the metadata and
		// the accessor is a symbol that library exports. What this
		// module's own records point at is that module's full record.
		if chain := nominalChain(t); len(chain) > 0 {
			if mangled, err := mangle.NominalType(mangle.Decl{
				Module: g.moduleOfType(t), Context: chain, ModuleOf: g.moduleOfType,
			}); err == nil {
				g.m.Metadata(sil.TypeMetadata{
					Type: typeNameOf(t), Mangled: mangled, Name: chain[len(chain)-1].Name,
					Module: g.moduleOfType(t), Layout: t, Imported: true,
				})
			}
		}
		return true
	}
	chain := nominalChain(t)
	if len(chain) == 0 {
		g.refuse(at, "a value of '"+typeName(t)+"' where its type has to be named at "+
			"run time, and this compiler has no name for it")
		return false
	}
	// Once is enough: a recursive enum's payload names the enum again.
	if meta, done := g.m.MetadataFor(typeNameOf(t)); done {
		if g.publicTypes != nil && g.publicTypes[typeNameOf(t)] && !meta.Public {
			meta.Public = true
			g.m.Metadata(meta)
		}
		return true
	}
	// Named as the type's own module, which is the module being compiled
	// for a type declared here and the standard library for a core one.
	// Every reference to the accessor names it that way too, and the two
	// have to agree or the record is emitted under a symbol nothing asks
	// for.
	mangled, err := mangle.NominalType(mangle.Decl{
		Module:   g.moduleOfType(t),
		Context:  chain,
		ModuleOf: g.moduleOfType,
	})
	if err != nil {
		g.refuse(at, "a value of '"+typeName(t)+"' where its type has to be named at "+
			"run time: "+err.Error())
		return false
	}
	modSym, err := mangle.ModuleDescriptor(g.module)
	if err != nil {
		g.refuse(at, "a module this compiler cannot name: "+err.Error())
		return false
	}
	g.m.Metadata(sil.TypeMetadata{
		Type:      typeNameOf(t),
		Mangled:   mangled,
		Name:      chain[len(chain)-1].Name,
		Module:    g.module,
		ModuleSym: modSym,
		Layout:    t,
		Public:    g.publicTypes[typeNameOf(t)],
	})
	// An enum's payloads are described as its fields are, where they can
	// be: quietly, since a payload with no metadata leaves just the case.
	if en, ok := t.Underlying().(*types.Enum); ok {
		for _, c := range en.Cases {
			if c == nil || c.AssociatedType == nil {
				continue
			}
			switch c.AssociatedType.Underlying().(type) {
			case *types.Optional, *types.Array, *types.Dictionary, *types.Set, *types.Tuple:
				if g.quietlyDescribable(c.AssociatedType) {
					g.structuralMetadata(at, c.AssociatedType)
				}
			case *types.Struct, *types.Enum:
				if len(nominalChain(c.AssociatedType)) > 0 {
					if _, done := g.m.MetadataFor(typeNameOf(c.AssociatedType)); !done {
						g.needMetadata(at, c.AssociatedType)
					}
				}
			}
		}
	}
	// Emit nested metadata for struct field types.
	if st, ok := t.Underlying().(*types.Struct); ok {
		for _, f := range st.Fields {
			if f == nil || f.Type == nil {
				continue
			}
			switch f.Type.Underlying().(type) {
			case *types.Optional, *types.Array, *types.Dictionary, *types.Set, *types.Tuple:
				// Quietly: a field whose type cannot be described leaves
				// the struct described without its fields.
				if g.quietlyDescribable(f.Type) {
					g.structuralMetadata(at, f.Type)
				}
				continue
			case *types.Struct, *types.Enum:
			default:
				continue
			}
			if len(nominalChain(f.Type)) == 0 {
				continue
			}
			if _, done := g.m.MetadataFor(typeNameOf(f.Type)); !done {
				g.needMetadata(at, f.Type)
			}
		}
	}
	return true
}

// conformanceSymbols returns the mangled witness table and conformance descriptor symbols.
func (g *gen) conformanceSymbols(at ast.Node, concrete types.Type,
	p *types.Protocol) (string, string, bool) {
	chain := nominalChain(concrete)
	extended := extendedBuiltin(concrete)
	if (chain == nil && extended == nil) || p == nil {
		return "", "", false
	}
	c := mangle.Conf{
		Type: mangle.Decl{
			Module:   g.moduleOfType(concrete),
			Context:  chain,
			Extended: extended,
			ModuleOf: g.moduleOfType,
		},
		ProtocolModule: g.moduleOfType(p),
		ProtocolName:   p.Name,
		Module:         g.module,
	}
	table, err := mangle.WitnessTable(c)
	if err != nil {
		g.refuse(at, "a conformance this compiler cannot name: "+err.Error())
		return "", "", false
	}
	descr, err := mangle.ConformanceDescriptor(c)
	if err != nil {
		g.refuse(at, "a conformance this compiler cannot name: "+err.Error())
		return "", "", false
	}
	return table, descr, true
}

// writesWitnesses reports whether value witness tables are generated by this module.
func (g *gen) writesWitnesses(t types.Type) bool {
	if t == nil {
		return false
	}
	if len(nominalChain(t)) == 0 {
		return false
	}
	return g.moduleOfType(t) == g.module
}

// ownsOnlyReferences reports whether all owned fields are retainable reference types.
func ownsOnlyReferences(t types.Type) bool {
	st, ok := t.Underlying().(*types.Struct)
	if !ok {
		return false
	}
	for _, f := range st.Fields {
		if f == nil || f.Type == nil {
			return false
		}
		switch u := f.Type.Underlying().(type) {
		case *types.Basic:
			if u.Kind() != types.String && !lowerType(f.Type).Trivial() {
				return false
			}
		case *types.Array, *types.Dictionary, *types.Set, *types.Class:
		case *types.Struct:
			if !lowerType(f.Type).Trivial() && !ownsOnlyReferences(f.Type) {
				return false
			}
		case *types.Enum:
			if !lowerType(f.Type).Trivial() && !enumOwnsOnlyReferences(f.Type) {
				return false
			}
		default:
			if !lowerType(f.Type).Trivial() {
				return false
			}
		}
	}
	return true
}

// structuralMetadata records that this module emits metadata for an
// Optional or an Array, and answers the record's symbol, without its
// suffix. What it wraps or holds needs metadata too, and gets it here.
func (g *gen) structuralMetadata(at ast.Node, t types.Type) (string, bool) {
	var inner []types.Type
	switch u := t.Underlying().(type) {
	case *types.Optional:
		inner = []types.Type{u.Wrapped}
	case *types.Array:
		inner = []types.Type{u.Elem}
	case *types.Dictionary:
		inner = []types.Type{u.Key, u.Value}
	case *types.Set:
		inner = []types.Type{u.Elem}
	case *types.Tuple:
		if len(u.Elements) == 0 {
			return "", false
		}
		for _, el := range u.Elements {
			inner = append(inner, el.Type)
		}
	default:
		return "", false
	}
	for _, x := range inner {
		if !g.describable(at, x) {
			return "", false
		}
	}
	mangled := "$sVSCmeta_" + identifierSafe(t.String())
	if _, done := g.m.MetadataFor(sil.StructuralKey(t)); done {
		return mangled, true
	}
	modSym, err := mangle.ModuleDescriptor(g.module)
	if err != nil {
		g.refuse(at, "a module this compiler cannot name: "+err.Error())
		return "", false
	}
	g.m.Metadata(sil.TypeMetadata{
		Type:      sil.StructuralKey(t),
		Mangled:   mangled,
		Name:      t.String(),
		Module:    g.module,
		ModuleSym: modSym,
		Layout:    t,
	})
	return mangled, true
}

// describable reports whether a type has metadata this module can point
// at, arranging for it where this module is the one to emit it.
func (g *gen) describable(at ast.Node, t types.Type) bool {
	if t == nil {
		return false
	}
	if ex, ok := existentialOf(t); ok && len(ex.Protocols) <= 1 {
		return true
	}
	if _, ok := t.(*types.Metatype); ok {
		return true
	}
	switch u := t.Underlying().(type) {
	case *types.Basic:
		if _, ok := metadataRecords[u.Kind()]; ok {
			return true
		}
	case *types.Optional, *types.Array, *types.Dictionary, *types.Set:
		_, ok := g.structuralMetadata(at, t)
		return ok
	case *types.Tuple:
		// The empty tuple is Void, which no value is.
		if len(u.Elements) == 0 {
			break
		}
		_, ok := g.structuralMetadata(at, t)
		return ok
	case *types.Struct, *types.Class, *types.Enum:
		if len(nominalChain(t)) == 0 {
			break
		}
		return g.needMetadata(at, t)
	}
	g.refuse(at, "the metadata for '"+t.String()+"', which this compiler cannot name")
	return false
}

// quietlyDescribable is describable without the refusal: whether every
// type an Optional or Array field is made of has metadata.
func (g *gen) quietlyDescribable(t types.Type) bool {
	switch u := t.Underlying().(type) {
	case *types.Basic:
		_, ok := metadataRecords[u.Kind()]
		return ok
	case *types.Optional:
		return g.quietlyDescribable(u.Wrapped)
	case *types.Array:
		return g.quietlyDescribable(u.Elem)
	case *types.Dictionary:
		return g.quietlyDescribable(u.Key) && g.quietlyDescribable(u.Value)
	case *types.Set:
		return g.quietlyDescribable(u.Elem)
	case *types.Tuple:
		for _, el := range u.Elements {
			if !g.quietlyDescribable(el.Type) {
				return false
			}
		}
		return len(u.Elements) > 0
	case *types.Struct, *types.Enum, *types.Class:
		return len(nominalChain(t)) > 0
	}
	return false
}

// enumOwnsOnlyReferences reports whether t is an enum each of whose cases
// carries nothing counted, or only references and Strings -- directly, or
// in a struct or tuple -- which the value witness table lowering writes
// for it retains and releases case by case.
func enumOwnsOnlyReferences(t types.Type) bool {
	e, ok := t.Underlying().(*types.Enum)
	if !ok {
		return false
	}
	for _, k := range e.Cases {
		if k == nil || k.AssociatedType == nil || lowerType(sil.CaseStorage(k)).Trivial() {
			continue
		}
		if !payloadOwnsOnlyReferences(sil.CaseStorage(k)) {
			return false
		}
	}
	return true
}

// payloadOwnsOnlyReferences is the same question of what one case carries.
func payloadOwnsOnlyReferences(t types.Type) bool {
	switch u := t.Underlying().(type) {
	case *types.Basic:
		return u.Kind() == types.String || lowerType(t).Trivial()
	case *types.Array, *types.Dictionary, *types.Set, *types.Class, *sil.BoxType:
		return true
	case *types.Struct:
		return lowerType(t).Trivial() || ownsOnlyReferences(t)
	case *types.Tuple:
		for _, el := range u.Elements {
			if !payloadOwnsOnlyReferences(el.Type) {
				return false
			}
		}
		return true
	}
	return lowerType(t).Trivial()
}

// isOperatorName reports whether a name is an operator's.
func isOperatorName(name string) bool {
	if name == "" {
		return false
	}
	for i := 0; i < len(name); i++ {
		if !strings.ContainsRune("&@/=><*!|+?%-~^.", rune(name[i])) {
			return false
		}
	}
	return true
}

// getterWitnessThunk emits the row for a property requirement: a witness
// that takes the conformer by address and answers the property's value,
// read through its getter or, for a stored property, out of the value.
func (g *gen) getterWitnessThunk(concrete types.Type, p *types.Protocol, r *types.Requirement) (string, bool) {
	var computed, stored *types.Field
	if f, ok := g.computedField(concrete, r.Name); ok {
		computed = f
	} else if st, ok := concrete.Underlying().(*types.Struct); ok {
		for _, f := range st.Fields {
			if f.Name == r.Name {
				stored = f
			}
		}
	} else if cl, ok := concrete.Underlying().(*types.Class); ok {
		for _, f := range cl.Fields {
			if f.Name == r.Name {
				stored = f
			}
		}
	}
	if computed == nil && stored == nil {
		return "", false
	}
	field := computed
	if field == nil {
		field = stored
	}
	getter, err := mangle.Getter(mangle.Decl{
		Module:    g.memberModule(concrete, field),
		Context:   memberChain(concrete),
		Extended:  extendedBuiltin(concrete),
		Name:      field.Name,
		Signature: &types.Signature{Results: field.Type},
		ModuleOf:  g.moduleOfType,
	})
	if err != nil {
		return "", false
	}
	name := getter + "TW" + identifierSafe(typeNameOf(concrete)) + "_" + identifierSafe(p.Name)
	if existing := g.m.Lookup(name); existing != nil && !existing.IsDeclaration() {
		return name, true
	}
	f := g.m.Func(name).SetSourceName(r.Name).SetLinkage(sil.Private).SetAttr("ossa")

	outerFn, outerEntry, outerBlk := g.fn, g.entry, g.blk
	defer func() { g.fn, g.entry, g.blk = outerFn, outerEntry, outerBlk }()
	g.fn, g.entry = f, false
	f.Type().Params = nil
	g.blk = f.Entry()

	ct := lowerType(concrete)
	rt := lowerType(field.Type)
	selfAddr := f.Param(ct.Address(), sil.ParamInGuaranteed)
	f.Type().Convention = sil.ConvWitness
	f.SetResult(rt, resultConvention(rt))

	self := g.blk.Load(selfAddr, loadQualifier(ct))
	var out *sil.Value
	if computed != nil {
		callee := g.m.Func(getter).SetSourceName(field.Name)
		if g.needsType(callee) {
			callee.Type().Params = append(callee.Type().Params,
				sil.Param{Type: ct, Convention: selfConvention(ct)})
			callee.Type().Convention = sil.Method
			callee.SetResult(rt, resultConvention(rt))
		}
		out = g.blk.Apply(g.blk.FunctionRef(callee), rt, self)
	} else if isClass(concrete) {
		borrowed := g.blk.BeginBorrow(self)
		fieldAddr := g.blk.RefElementAddr(borrowed, memberName(concrete, field.Name), rt)
		access := g.blk.BeginAccess(fieldAddr, "read", "dynamic")
		out = g.blk.Load(access, loadQualifier(rt))
		g.blk.EndAccess(access)
		g.blk.EndBorrow(borrowed)
	} else if ct.Trivial() {
		out = g.blk.StructExtract(self, memberName(concrete, field.Name), rt)
	} else {
		borrowed := g.blk.BeginBorrow(self)
		out = g.blk.CopyValue(g.blk.StructExtract(borrowed, memberName(concrete, field.Name), rt))
		g.blk.EndBorrow(borrowed)
	}
	if !ct.Trivial() {
		g.blk.DestroyValue(self)
	}
	g.blk.Return(out)
	return name, true
}

// staticGetterWitnessThunk emits the row for a static property
// requirement -- CaseIterable's allCases -- which takes the conformer's
// type in the self register and answers what its static property holds:
// through the static getter of a computed one, out of the storage of a
// stored one.
func (g *gen) staticGetterWitnessThunk(concrete types.Type, p *types.Protocol, r *types.Requirement) (string, bool) {
	var field *types.Field
	for _, f := range g.staticsOf(concrete) {
		if f != nil && f.Name == r.Name {
			field = f
		}
	}
	if field == nil {
		return "", false
	}
	d := mangle.Decl{
		Module:    g.memberModule(concrete, field),
		Context:   memberChain(concrete),
		Extended:  extendedBuiltin(concrete),
		Name:      field.Name,
		Signature: &types.Signature{Results: field.Type},
		ModuleOf:  g.moduleOfType,
	}
	getter, err := mangle.StaticGetter(d)
	if err != nil {
		return "", false
	}
	name := getter + "TW" + identifierSafe(typeNameOf(concrete)) + "_" + identifierSafe(p.Name)
	if existing := g.m.Lookup(name); existing != nil && !existing.IsDeclaration() {
		return name, true
	}
	f := g.m.Func(name).SetSourceName(r.Name).SetLinkage(sil.Private).SetAttr("ossa")
	outerFn, outerEntry, outerBlk := g.fn, g.entry, g.blk
	defer func() { g.fn, g.entry, g.blk = outerFn, outerEntry, outerBlk }()
	g.fn, g.entry = f, false
	f.Type().Params = nil
	g.blk = f.Entry()

	rt := lowerType(field.Type)
	f.Param(sil.ThickMetatype(concrete), sil.ParamUnowned)
	f.Type().Convention = sil.ConvWitness
	f.SetResult(rt, resultConvention(rt))
	var out *sil.Value
	if field.IsComputed {
		g.emitCoreGetter(concrete, field, getter, true)
		callee := g.m.Func(getter).SetSourceName(field.Name)
		if g.needsType(callee) {
			callee.Type().Params = nil
			callee.SetResult(rt, resultConvention(rt))
		}
		out = g.blk.Apply(g.blk.FunctionRef(callee), rt)
	} else {
		addr := g.staticAddr(nil, concrete, field)
		if addr == nil {
			return "", false
		}
		access := g.blk.BeginAccess(addr, "read", "dynamic")
		out = g.blk.Load(access, loadQualifier(rt))
		g.blk.EndAccess(access)
	}
	g.blk.Return(out)
	return name, true
}

// requiresSelfOperands reports whether a requirement is a static operator
// on the conformer -- Equatable's `==`, Comparable's `<` -- whose witness
// takes the operands by address and the conformer's type in self.
func requiresSelfOperands(p *types.Protocol, r *types.Requirement) bool {
	return r.Sig != nil && isOperatorName(r.Name) && len(r.Sig.Params) > 0
}

// staticWitnessThunk emits the row for a static operator requirement: the
// operands arrive by address, as a generic's values of Self do, the type in
// the self register, and the conformer's own operator answers -- the one it
// declares, the one derived for it, or for an enum of cases alone, a
// comparison of the tags.
func (g *gen) staticWitnessThunk(concrete types.Type, p *types.Protocol, r *types.Requirement) (string, bool) {
	name := "$sWS" + identifierSafe(r.Name) + "TW" + identifierSafe(typeNameOf(concrete)) + "_" + identifierSafe(p.Name)
	if existing := g.m.Lookup(name); existing != nil && !existing.IsDeclaration() {
		return name, true
	}
	subst := map[*types.TypeParam]types.Type{p.Self: concrete}
	want, _ := types.Substitute(r.Sig, subst).(*types.Signature)
	if want == nil {
		return "", false
	}

	// What answers it.
	var ref *analyzer.MethodRef
	recv, methods := staticMethodsNamed(concrete, r.Name)
	if r.Name == "==" && len(methods) == 0 {
		recv, methods = staticMethodsNamed(concrete, derive.EqualsName)
	}
	if r.Name == "==" && len(methods) == 0 {
		recv, methods = staticMethodsNamed(concrete, derive.EnumEqualsName)
	}
	for _, m := range methods {
		if m.Sig != nil && types.Identical(m.Sig, want) {
			ref = &analyzer.MethodRef{Recv: recv, Method: m}
			break
		}
	}
	en, isEnum := enumFor(concrete)
	if ref == nil && !(isEnum && r.Name == "==" && !hasPayloadCase(en)) {
		return "", false
	}

	f := g.m.Func(name).SetSourceName(r.Name).SetLinkage(sil.Private).SetAttr("ossa")
	outerFn, outerEntry, outerBlk := g.fn, g.entry, g.blk
	defer func() { g.fn, g.entry, g.blk = outerFn, outerEntry, outerBlk }()
	g.fn, g.entry = f, false
	f.Type().Params = nil
	g.blk = f.Entry()

	var addrs []*sil.Value
	var vals []*sil.Value
	for i, param := range want.Params {
		t := lowerType(param.Type)
		if r.Sig.Params[i].Type == types.Type(p.Self) {
			a := f.Param(t.Address(), sil.ParamInGuaranteed)
			addrs = append(addrs, a)
			vals = append(vals, g.blk.Load(a, loadQualifier(t)))
			continue
		}
		vals = append(vals, f.Param(t, paramConvention(param, t)))
	}
	f.Param(sil.ThickMetatype(concrete), sil.ParamUnowned)
	f.Type().Convention = sil.ConvWitness
	rt := lowerType(want.Results)
	f.SetResult(rt, resultConvention(rt))

	var out *sil.Value
	if ref != nil {
		callee := g.m.Func(g.staticSymbol(ref, concrete)).SetSourceName(ref.Method.Name)
		if g.needsType(callee) {
			g.declareSignature(callee, ref.Method.Sig)
		}
		out = g.blk.Apply(g.blk.FunctionRef(callee), rt, vals...)
	} else {
		raw := g.blk.Builtin("cmp_eq_"+enumMachine(en), sil.Object(sil.BuiltinInt1), vals[0], vals[1])
		out = g.blk.Struct(rt, raw)
	}
	for i, a := range addrs {
		if lt := lowerType(want.Params[i].Type); !lt.Trivial() {
			_ = a
			g.blk.DestroyValue(vals[i])
		}
	}
	g.blk.Return(out)
	return name, true
}

// hasPayloadCase reports whether any case of an enum carries a value.
func hasPayloadCase(e *types.Enum) bool {
	for _, c := range e.Cases {
		if c != nil && c.AssociatedType != nil {
			return true
		}
	}
	return false
}

// staticMethodsNamed is a type's static methods of a name, and the type
// declaring them.
func staticMethodsNamed(t types.Type, name string) (types.Type, []*types.Method) {
	if inst, ok := t.(*types.GenericInstance); ok {
		t = inst.Base
	}
	var methods []*types.Method
	switch b := t.Underlying().(type) {
	case *types.Struct:
		methods = b.Methods
	case *types.Class:
		methods = b.Methods
	case *types.Enum:
		methods = b.Methods
	}
	var out []*types.Method
	for _, m := range methods {
		if m != nil && m.IsStatic && m.Name == name {
			out = append(out, m)
		}
	}
	return t, out
}

// typeParamsOfType is a nominal type's own generic parameters.
func typeParamsOfType(t types.Type) []*types.TypeParam {
	switch n := t.Underlying().(type) {
	case *types.Struct:
		return n.TypeParams
	case *types.Enum:
		return n.TypeParams
	case *types.Class:
		return n.TypeParams
	}
	return nil
}

// ownedExistential is an existential a use may take: a temporary as it is,
// and a variable's own storage copied out of into a temporary of its own,
// which counts whatever is inside a second time.
func (g *gen) ownedExistential(v *sil.Value) *sil.Value {
	if v == nil || !g.storage[v] {
		return v
	}
	slot := g.blk.AllocStack(v.Type().Object())
	g.blk.CopyAddr(v, slot, "init")
	return slot
}

// intoExistential initializes the existential storage at slot from an
// expression that is one already: copied out of a variable, taken over
// from a temporary.
func (g *gen) intoExistential(src *sil.Value, slot *sil.Value) {
	if g.storage[src] {
		g.blk.CopyAddr(src, slot, "init")
		return
	}
	g.blk.CopyAddr(src, slot, "take", "init")
}

// methodMatching is the method a concrete type declares that meets a
// requirement: of its name, taking the same labels and parameter types --
// so that of two overloads, `Write(_: [UInt8])` and `Write(_: String)`,
// the one the requirement names is the one in the table. It falls back to
// the first of the name where none matches exactly.
func (g *gen) methodMatching(t types.Type, r *types.Requirement) (types.Type, *types.Method) {
	found, first := g.methodOn(t, r.Name)
	if first == nil || r.Sig == nil {
		return found, first
	}
	base := t
	if inst, ok := base.(*types.GenericInstance); ok {
		base = inst.Underlying()
	}
	var methods []*types.Method
	switch b := base.Underlying().(type) {
	case *types.Struct:
		methods = b.Methods
	case *types.Class:
		methods = b.Methods
	case *types.Enum:
		methods = b.Methods
	}
	for _, m := range methods {
		if m == nil || m.Name != r.Name || m.Sig == nil || len(m.Sig.Params) != len(r.Sig.Params) {
			continue
		}
		same := true
		for i, p := range m.Sig.Params {
			q := r.Sig.Params[i]
			if p.Label != q.Label || !types.Identical(p.BodyType(), q.BodyType()) {
				same = false
				break
			}
		}
		if same {
			return found, m
		}
	}
	return found, first
}

// existentialPlace is where an existential expression's container is: a
// variable's storage, or a temporary in memory ended after the statement.
func (g *gen) existentialPlace(x ast.Expr) *sil.Value {
	if addr := g.lvalue(x); addr != nil {
		return addr
	}
	// Indirect existential return values already reside in allocated memory.
	if _, isCall := x.(*ast.CallExpr); isCall {
		addr := g.rvalue(x)
		g.destroyLater(addr)
		if addr != nil {
			return addr
		}
		g.refuse(x, "an existential that is not somewhere in memory")
		return nil
	}
	v := g.expr(x)
	switch {
	case v == nil:
		return nil
	case v.Type().IsAddress():
		// Any other existential expression -- an element read out of an
		// array -- is a temporary in memory, ended after the statement.
		if !g.storage[v] {
			g.destroyAddrLater(v)
		}
		return v
	}
	// An existential held as a value -- a closure's parameter -- is put in
	// a temporary of its own, a copy where the value is borrowed.
	lt := v.Type()
	slot := g.blk.AllocStack(lt)
	g.blk.Store(g.consume(v), slot, storeQualifier(lt))
	g.destroyAddrLater(slot)
	return slot
}

// conformsToAll reports whether t conforms to every protocol of ex by what
// it is rather than by what it declares: a class to AnyObject.
func conformsToAll(t types.Type, ex *types.Existential) bool {
	for _, p := range ex.Protocols {
		if !types.ConformsTo(t, p) {
			return false
		}
	}
	return true
}
