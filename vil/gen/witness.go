package gen

import (
	"github.com/vertex-language/vsc/analyzer"
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/mangle"
	"github.com/vertex-language/vsc/types"
	"github.com/vertex-language/vsc/vil"
)

// Existentials, and the witness tables they carry.
//
// A constrained generic knows its type argument, so a call through a
// constraint is a lookup at compile time -- see generic.go. An
// existential is the case where that is not true: `any P` is a value
// whose type arrives with it, so which implementation runs cannot be
// decided until it does. That is what a witness table is for, and it
// is why this is a runtime table where the constrained case needed
// none.
//
// swiftc's shape, which this follows:
//
//	sil_witness_table hidden A: P module ex {
//	  method #P.v: ... : @$s2ex1AVAA1PA2aDP1vs5Int32VyFTW
//	}
//
// One table per conformance, one row per requirement, and each row
// naming a thunk rather than the method itself. The thunk is not
// ceremony: a witness is called with its receiver by address, because
// the caller has an existential and not a value of the type, while
// the method wants the value. The thunk is where the one becomes the
// other, and it is the only place that knows which type it is.
//
// # The layout
//
// Five words, which is what types.Sizeof says an existential of one
// protocol is and what Swift uses: three of buffer, then the
// metadata, then the witness table.
//
// A table is the conformance descriptor, then the table of each
// protocol this one inherits, then a row per requirement in the order
// the protocol declares them. So a requirement's row is one past its
// place in the protocol, and a requirement a protocol inherits is two
// loads rather than one -- both of which are swiftc's layout read off
// its own output rather than a convention chosen here. The descriptor
// word is null: this compiler does not emit one yet, which is what
// keeps a table it wrote from being one a Swift library can read.
//
// A concrete type larger than the buffer would have to be boxed on
// the heap. Swift does; this does not, when it is the one filling the
// existential in, and such a value is refused rather than written
// past the end of the buffer. Reading one is a different matter --
// see openExistential in lower, which asks the metadata where the
// value is rather than assuming.
//
// # What one may hold
//
// Only a value that owns nothing. Swift copies and destroys what is
// inside an existential through its value witness table, which is how
// either can be done without knowing what is in there; this emits no
// such table, so a value that owns something would be copied without
// a retain and dropped without a release. A class in an existential
// is refused for that reason and not because it is hard.
//
// That restriction is what the rest of this leans on. Every
// existential in a module this compiler accepts holds a trivial
// value, so an existential is forty bytes that can be copied by
// copying them -- which is why an existential variable can be
// assigned to, and why nothing is destroyed before it is.
//
// # Where one lives
//
// In memory, always. An existential has no register form: a binding
// of that type is a slot allocated where the binding begins, a
// parameter arrives by address and stays one, and a use of either
// hands over the address rather than loading five words out of it.
// SILGen does the same, and for the same reason -- there is nothing
// to load.
//
// One thing follows that this refuses rather than lowers: a type with
// a stored property of existential type is held in memory itself,
// while this builds every struct in registers. It is refused where it
// is declared, so that the message names the declaration rather than
// some expression three files away.
//
// A function whose result is an existential fills in storage the
// caller set aside -- `-> @out P` in SIL, sret in the object file.
// One declared here is still refused, because what it would return is
// a table this compiler named its own way. One imported is not: what
// comes back was built by swiftc, and reading it is agreeing with a
// layout rather than producing one.

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
			for _, p := range conformancesOf(sym.Type()) {
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
	// The module is what says whether this has been done, rather than
	// anything on this gen: a module is lowered by one gen per file
	// and one more for the tables, and a map on any of them knows
	// only what that one did. Emitting a table twice appends its
	// thunk's parameters twice.
	if hasWitnessTable(g.m, typeNameOf(concrete), p.Name) {
		return
	}
	if _, ok := g.layoutOrder(at, p); !ok {
		return
	}
	// A conformance to a protocol that inherits another is two
	// conformances: swiftc emits a table for each, and the derived
	// one holds the base one in a row. So the base tables come first,
	// and then the row that names each.
	for _, up := range p.Inherited {
		if up != nil {
			g.witnessTable(at, concrete, up)
		}
	}
	table := g.m.WitnessTable(typeNameOf(concrete), p.Name, g.module, vil.Hidden)
	if sym, descr, ok := g.conformanceSymbols(at, concrete, p); ok {
		table.Symbol, table.Descriptor = sym, descr
		// The protocol's own descriptor, which the conformance points
		// at. Only one another module declared: this compiler emits
		// none for a protocol declared here.
		if m := g.moduleOfType(p); m != "" && m != g.module {
			if s, err := mangle.ProtocolDescriptor(m, p.Name); err == nil {
				table.ProtocolSymbol = s
			}
		}
	}
	for _, up := range p.Inherited {
		if up != nil {
			table.Entry(baseRow(p, up), up.Name)
		}
	}
	for _, r := range p.Requirements {
		if r == nil || r.Sig == nil {
			continue
		}
		found, m := methodOn(concrete, r.Name)
		if m == nil {
			g.errorAt(at, "'"+typeNameOf(concrete)+"' does not provide '"+
				r.Name+"', which '"+p.Name+"' requires")
			continue
		}
		thunk := g.witnessThunk(concrete, found, p, m)
		if thunk == "" {
			continue
		}
		table.Entry(p.Name+"."+r.Name, thunk)
	}
}

// witnessThunk emits the function a table row names: the receiver
// arrives as an address, and what the method wants is the value.
func (g *gen) witnessThunk(concrete, found types.Type, p *types.Protocol, m *types.Method) string {
	name := witnessThunkSymbol(concrete, p, m)
	if existing := g.m.Lookup(name); existing != nil && !existing.IsDeclaration() {
		return name
	}
	f := g.m.Func(name).SetSourceName(m.Name).SetLinkage(vil.Hidden).SetAttr("ossa")

	outer := struct {
		fn      *vil.Func
		entry   bool
		blk     *vil.Block
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
	var args []*vil.Value
	for i, param := range m.Sig.Params {
		t := lowerType(param.Type)
		v := f.Param(t, paramConvention(param, t))
		args = append(args, v)
		if param.Name != "" {
			g.blk.DebugValue(v, param.Name, "let", "argno "+itoa(i+1))
		}
	}
	selfAddr := f.Param(ct.Address(), vil.ParamInGuaranteed)
	f.Type().Convention = vil.ConvWitness
	if m.Sig.Results != nil && !isVoid(m.Sig.Results) {
		rt := lowerType(m.Sig.Results)
		f.SetResult(rt, resultConvention(rt))
	}

	self := g.blk.Load(selfAddr, loadQualifier(ct))
	callee := g.m.Func(g.methodSymbol(&analyzer.MethodRef{Recv: found, Method: m})).
		SetSourceName(m.Name)
	if g.needsType(callee) {
		g.declareMethod(callee, &analyzer.MethodRef{Recv: found, Method: m})
	}
	ref := g.blk.FunctionRef(callee)
	args = append(args, self)

	if m.Sig.Results == nil || isVoid(m.Sig.Results) {
		g.blk.Apply(ref, lowerType(m.Sig.Results), args...)
		g.blk.Return(g.void())
		return name
	}
	g.blk.Return(g.blk.Apply(ref, lowerType(m.Sig.Results), args...))
	return name
}

// existentialCall calls a requirement through the table the value
// carries.
func (g *gen) existentialCall(e *ast.CallExpr, ref *analyzer.MethodRef, mem *ast.MemberExpr, ex *types.Existential) *vil.Value {
	p, path, ok := protocolProviding(ex, ref.Method.Name)
	if !ok {
		g.refuse(e, "a call to something no protocol of this existential promises")
		return nil
	}
	// The row this call reads is the requirement's place in the
	// protocol, so the protocol has to have said what that order is --
	// including when the table it will be read out of belongs to a
	// library and not to this module.
	if _, ok := g.layoutOrder(e, p); !ok {
		return nil
	}
	addr := g.lvalue(mem.X)
	if addr == nil {
		// One handed back by a call is in memory too: an existential
		// is five words and comes back through storage the caller set
		// aside, which is an address already.
		if _, isCall := mem.X.(*ast.CallExpr); isCall {
			addr = g.rvalue(mem.X)
			// The storage the call filled in belongs to this
			// statement, and what is in it is destroyed when the
			// statement ends.
			g.destroyLater(addr)
		}
	}
	if addr == nil {
		g.refuse(mem.X, "an existential that is not somewhere in memory")
		return nil
	}

	var args []*vil.Value
	want := existentialParams(ref.Method.Sig)
	if e.Args != nil {
		for i, a := range e.Args.Args {
			v := g.rvalue(a.X)
			if v == nil {
				return nil
			}
			args = append(args, g.boxArg(a.X, v, want, i))
		}
	}

	method := g.blk.WitnessMethodOn(addr, witnessMember(path, ref.Method.Name),
		witnessType(ref.Method, ex))
	opened := g.blk.OpenExistentialAddr(addr, lowerType(ex).Address())
	args = append(args, opened)

	v := g.blk.Apply(method, lowerType(ref.Method.Sig.Results), args...)
	g.destroyLater(v)
	return v
}

// witnessType is the function type a witness has: the declared
// parameters, then the receiver as an address.
func witnessType(m *types.Method, ex *types.Existential) vil.Type {
	ft := &vil.FuncType{Convention: vil.ConvWitness}
	for _, p := range m.Sig.Params {
		t := lowerType(p.Type)
		ft.Params = append(ft.Params, vil.Param{Type: t, Convention: paramConvention(p, t)})
	}
	ft.Params = append(ft.Params, vil.Param{
		Type:       lowerType(ex).Address(),
		Convention: vil.ParamInGuaranteed,
	})
	if m.Sig.Results != nil && !isVoid(m.Sig.Results) {
		t := lowerType(m.Sig.Results)
		ft.Results = append(ft.Results, vil.Result{Type: t, Convention: resultConvention(t)})
	}
	return vil.Object(ft)
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

// layoutOrder is the order a protocol's requirements sit in a witness
// table, recorded on the module so that a call through an existential
// can find one by name whether or not the table is in this module.
//
// swiftc's order is the protocol's own requirements in the order it
// declares them, after one row for each protocol it inherits: the
// table for `Sub: Base` holds the descriptor, then the table for
// Base, then Sub's own. This compiler writes the first and the last
// of those three and not the middle, so a protocol that inherits
// another is refused by name rather than laid out one row short --
// which would run every call through the table off by one.
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
		if r == nil || r.Sig == nil {
			continue
		}
		names = append(names, p.Name+"."+r.Name)
	}
	g.m.Requirements(p.Name, names)
	return names, true
}

// baseRow names the row of a protocol's table that holds the table
// for a protocol it inherits.
//
// A colon rather than a dot, so that a row naming another table and a
// row naming a function are told apart by their shape: `Labelled.size`
// is a requirement and `Labelled:Sized` is a conformance.
func baseRow(p, up *types.Protocol) string { return p.Name + ":" + up.Name }

// witnessPath is the chain of tables a call walks to reach the
// implementation of one requirement, from the protocol the
// existential names to the protocol that declares it.
//
// One protocol deep is the ordinary case and the chain is just that
// protocol. Where the requirement is inherited the chain has the step
// in it, because that is what the table says: swiftc's own code for
// `x.b()` through `any Sub` loads Base's table out of Sub's, and then
// b out of Base's.
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
func conformancesOf(t types.Type) []*types.Protocol {
	if t == nil {
		return nil
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

// existentialOf is the existential a type is, if it is one.
//
// Both spellings arrive here. `any P` resolves to an Existential; a
// bare `P` in type position resolves to the protocol itself, and in
// value position that means the same thing -- a value that satisfies
// P, whose type is not known until it arrives.
func existentialOf(t types.Type) (*types.Existential, bool) {
	switch n := t.(type) {
	case *types.Existential:
		// `Any` promises nothing, so there is no requirement to look
		// up and no table to look one up in. What it carries instead
		// is the metadata that says what is inside it -- which is
		// four words rather than five, and is an existential like any
		// other here.
		return n, true
	case *types.Protocol:
		return &types.Existential{Protocols: []*types.Protocol{n}}, true
	}
	return nil, false
}

// existentialFor wraps a concrete value in an existential, or hands
// back what it was given when no wrapping is called for.
//
// This is where a value becomes one whose type is not known: it is
// copied into the buffer, the table for its conformance goes beside
// it, and what the caller passes on is the address of the pair. SIL
// writes the same three steps -- alloc_stack, init_existential_addr,
// store -- and lowering fills the table in, because which table it is
// follows from the concrete type and the protocol and nothing at the
// call site has to say.
func (g *gen) existentialFor(at ast.Node, v *vil.Value, from, to types.Type) *vil.Value {
	ex, ok := existentialOf(to)
	if !ok || v == nil {
		return v
	}
	if _, already := existentialOf(from); already {
		return v
	}
	// A protocol with an associated type cannot be put in an
	// existential here. What Item is arrives with the value, and an
	// existential carries a buffer and a witness table and no
	// metadata -- so there is nowhere for the answer to travel, and a
	// call to a requirement that returns one would have no result
	// type. Swift carries the metadata and can; this says which
	// protocol and stops.
	for _, p := range ex.Protocols {
		if a, ok := associatedIn(p); ok {
			g.refuse(at, "a value in 'any "+p.Name+"': "+p.Name+" leaves '"+a+
				"' to the conforming type, and an existential here carries a witness "+
				"table but not the metadata that would say what it is")
			return v
		}
	}
	if len(ex.Protocols) > 0 && conformancesOf(from) == nil && from != nil {
		// Nothing declares a conformance for it, so there is no table
		// to put beside it. The checker reports the mismatch; this
		// does not invent a box. `Any` asks for no table and is not
		// this case.
		return v
	}
	slot := g.blk.AllocStack(lowerType(ex))
	if !g.initExistential(at, slot, v, from, ex) {
		return v
	}
	return slot
}

// initExistential writes a concrete value into existential storage
// that is not holding anything yet, and reports whether it could.
//
// It is separate from existentialFor because storage comes from more
// than one place: an argument's is a temporary made for the call, and
// a variable's is the slot the variable lives in for as long as it is
// in scope. What goes into it is the same either way.
func (g *gen) initExistential(at ast.Node, slot, v *vil.Value, from types.Type, ex *types.Existential) bool {
	// A value larger than the buffer would have to be boxed on the
	// heap, which Swift does and this does not. Refusing by name is
	// the point: writing it into the buffer would run off the end of
	// the existential and into whatever followed.
	if size := types.Sizeof(from, types.DefaultTarget64); size > existentialBufferWords*8 {
		g.refuse(at, "a value of "+size64(size)+" bytes in an existential, which holds "+
			size64(existentialBufferWords*8)+" inline and boxes the rest")
		return false
	}
	// What is inside an existential is copied and destroyed through
	// its value witness table. A type declared elsewhere brought its
	// own, written by whoever declared it, and it is right whatever
	// the value owns -- a String's releases the bridge object.
	//
	// A type declared here has the one this compiler emits, and that
	// one is memcpy and nothing else. So a value of such a type that
	// owns something would be copied without a retain and dropped
	// without a release, and it is refused: the restriction is on the
	// witnesses this compiler writes, not on existentials.
	if t := lowerType(from); !t.Trivial() && g.writesWitnesses(from) {
		g.refuse(at, "a "+typeNameOf(from)+" in an existential: what is inside one is "+
			"copied and destroyed through a value witness table, and the one this "+
			"compiler writes for a type declared here copies bytes and nothing else")
		return false
	}
	// The metadata that says what is in it. A value crossing into
	// Swift carries its type with it, and everything done with it
	// without knowing what it is goes through that record.
	meta, ok := g.stdlibMetadata(at, from)
	if !ok {
		return false
	}
	for _, p := range ex.Protocols {
		g.witnessTable(at, from, p)
	}
	buf := g.blk.InitExistentialAddr(slot, lowerType(from), meta)
	// The value is handed over: what owns it now is the storage, and
	// destroying it is the existential's business. A borrowed one is
	// copied first, the way any other hand-over copies it.
	g.blk.Store(g.consume(v), buf, storeQualifier(lowerType(from)))
	return true
}

// boxArg puts one argument into an existential where the parameter it
// is going into is one, and hands back what it was given otherwise.
//
// Every place a call's arguments are lowered goes through this, which
// is the point: a parameter declared `any P` is the same parameter
// whether the call reaches a function, a method, a witness or a
// specialization, and an argument that skipped the boxing at one of
// them would be read as an existential without being one.
func (g *gen) boxArg(a ast.Expr, v *vil.Value, want []types.Type, i int) *vil.Value {
	if i >= len(want) {
		return v
	}
	from := g.typeOf(a)
	// An optional wraps for the same reason an existential boxes: the
	// parameter is a different type from the argument, and something
	// has to make one into the other. See optional.go.
	v = g.optionalFor(a, v, from, want[i])
	return g.existentialFor(a, v, from, want[i])
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
func hasWitnessTable(m *vil.Module, typ, proto string) bool {
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

// needMetadata records that a type declared here has to be described
// at run time, and reports whether it can be.
//
// The names are settled here because mangling is this side's: what
// the type's symbol is, what the module's descriptor is called, and
// what the two are called in plain text. What the record is made of
// is layout, and lower answers that.
func (g *gen) needMetadata(at ast.Node, t types.Type) bool {
	if t == nil {
		return false
	}
	if m := g.moduleOfType(t); m != "" && m != g.module {
		// Declared elsewhere, so its module emitted the metadata and
		// the accessor is a symbol that library exports.
		return true
	}
	chain := nominalChain(t)
	if len(chain) == 0 {
		g.refuse(at, "a value of '"+typeName(t)+"' where its type has to be named at "+
			"run time, and this compiler has no name for it")
		return false
	}
	mangled, err := mangle.NominalType(mangle.Decl{
		Module:   g.module,
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
	g.m.Metadata(vil.TypeMetadata{
		Type:      typeNameOf(t),
		Mangled:   mangled,
		Name:      chain[len(chain)-1].Name,
		Module:    g.module,
		ModuleSym: modSym,
		Layout:    t,
	})
	return true
}

// conformanceSymbols are the two names a conformance has in the
// object file: the witness table and the record that describes it.
//
// swiftc's, read back from its own output: `$s3App4MineV3Lib8Measured`
// then `AAWP` or `AAMc`, where the protocol is written as its module
// and its name with no kind letter after it and the trailing `AA` is
// the module the conformance is declared in.
func (g *gen) conformanceSymbols(at ast.Node, concrete types.Type,
	p *types.Protocol) (string, string, bool) {
	chain := nominalChain(concrete)
	if chain == nil || p == nil {
		return "", "", false
	}
	c := mangle.Conf{
		Type: mangle.Decl{
			Module:   g.moduleOfType(concrete),
			Context:  chain,
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

// writesWitnesses reports whether the value witnesses for a type are
// the ones this compiler writes.
//
// A nominal type declared in this module has them: metadata.go emits
// a table of memcpy and nothing else. Everything else brought its own
// -- the standard library's for a String or an Int, and the declaring
// library's for a type from another module -- and those are right
// whatever the value owns.
func (g *gen) writesWitnesses(t types.Type) bool {
	if t == nil {
		return false
	}
	if len(nominalChain(t)) == 0 {
		return false
	}
	return g.moduleOfType(t) == g.module
}
