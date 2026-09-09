package lower

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/vertex-language/ir"
	"github.com/vertex-language/vsc/types"
	"github.com/vertex-language/vsc/vil"
)

// Options are what the caller decides about the translation.
type Options struct {
	// SymbolPrefix is what a name becomes in an object file: "_" on
	// Mach-O, empty on ELF and COFF.
	//
	// It is stated rather than read off the target, for the reason
	// vcc's lowering gives: the mapping belongs to the language and
	// not to the IR, nothing below this package renames a symbol, and
	// a platform that wants the underscore wants it on every symbol
	// the module defines or names. `swiftc -c` on Darwin writes
	// `_$s2m21fyS2iF` and `_main`, and a module built without the
	// prefix compiles and then fails to link, naming what it could
	// not find.
	SymbolPrefix string
}

// Module translates a lowered VIL module into a VIR module for the
// given target.
//
// The module must be in vil.StageLowered: this package translates the
// machine form of the program, and the ownership form is not it.
func Module(m *vil.Module, target ir.Target, opts Options) (*ir.Module, error) {
	if m.Stage() != vil.StageLowered {
		return nil, &Error{Err: ErrStage, What: string(m.Stage())}
	}
	l := &lowerer{
		out:    ir.NewModule(m.Name(), target),
		module: m,
		prefix: opts.SymbolPrefix,
		callee: make(map[string]ir.Callee),
		defs:   make(map[string]*ir.Func),
	}
	for _, f := range m.Funcs() {
		if err := l.declare(f); err != nil {
			return nil, err
		}
	}
	if err := l.vtables(m); err != nil {
		return nil, err
	}
	// Metadata comes before the tables, which point back at its
	// descriptor, and both come before any body: they are made of
	// function definitions, and a definition made while another one
	// is being filled in lands in the middle of that one.
	if err := l.allMetadata(m); err != nil {
		return nil, err
	}
	if err := l.witnessTables(m); err != nil {
		return nil, err
	}
	for _, f := range m.Funcs() {
		if f.IsDeclaration() {
			continue
		}
		if err := l.define(f); err != nil {
			return nil, err
		}
	}
	if err := l.out.Err(); err != nil {
		return nil, &Error{Err: ErrIR, What: err.Error()}
	}
	return l.out, nil
}

// A lowerer holds what is shared across the whole module.
type lowerer struct {
	out    *ir.Module
	prefix string
	callee map[string]ir.Callee
	defs   map[string]*ir.Func

	// witness is a conformance's table, by type and protocol, and
	// witnessRows is the order of a protocol's requirements -- which
	// every table for that protocol shares, because a call site knows
	// the protocol and not the type.
	witness     map[string]*ir.Global
	witnessRows map[string][]string

	// vtable is a class's dispatch table, and slots is the order of
	// its rows: a class_method knows the member it wants and the
	// static class it is calling on, and the index comes from
	// scanning that class's rows for it.
	vtable map[string]*ir.Global
	slots  map[string][]string

	// funcTypes are the VIR typedefs indirect calls name, one per
	// distinct Swift function type.
	funcTypes map[string]*ir.Type

	// sretTypes are the shapes of the storage indirect results are
	// written into, one per result type.
	sretTypes map[string]*ir.Type
	// meta is the metadata record for a type declared here, by the
	// type's name, and the pieces it is made of. See metadata.go.
	meta        map[string]*ir.Global
	vwts        map[string]*ir.Global
	descriptors map[string]*ir.Global
	strings2    map[string]*ir.Global
	witnessFns  map[string]*ir.Func
	records     map[string]*ir.GlobalImport
	// module is the VIL module being lowered, which is where the
	// mangled names metadata needs are recorded.
	module *vil.Module
	// swiftRelease is Swift's own release, which is what lets go of
	// the box an existential too large for its buffer keeps its value
	// in. It is not vertex_release: the box was made by Swift.
	swiftRelease ir.Callee
	// destroyWitness is the shape of the destroy value witness.
	destroyWitness *ir.Type
	// strings are the literals already placed, by their bytes, so
	// that one written twice is one copy.
	strings  map[string]*ir.Global
	literals int
	// bridgeRetain and bridgeRelease are Swift's own, which is what a
	// String's second word is counted by.
	bridgeRetain  ir.Callee
	bridgeRelease ir.Callee

	// accessors are the metadata accessors this module calls, by
	// symbol. See metadataAccessor.
	accessors map[string]ir.Callee

	// The runtime's three, declared the first time something needs
	// one. What they do is runtime/'s business — it builds them as a
	// VIR module of its own — and that they exist is ours.
	retain  ir.Callee
	release ir.Callee
	alloc   ir.Callee
}

// sym is the object-file name of a VIL name.
//
// Every symbol goes through here — the module's own functions, the
// externals it calls, and the runtime's retain and release — because
// the prefix is the platform's and the platform does not care which
// of them it is looking at. The map from VIL name to VIR symbol is
// this function and nothing else, so a caller that wants to find a
// definition by its VIL name looks it up before this is applied.
func (l *lowerer) sym(name string) string { return l.prefix + name }

// declare gives every VIL function a VIR symbol before any body is
// filled, so a call to a function defined later has something to name.
func (l *lowerer) declare(f *vil.Func) error {
	sig, err := l.signature(f.Name(), f.Type())
	if err != nil {
		return err
	}
	if f.IsDeclaration() {
		l.callee[f.Name()] = l.out.ImportFunc(l.sym(f.Name()), sig)
		return nil
	}
	out := l.out.Func(l.sym(f.Name()))
	l.defs[f.Name()] = out
	if err := l.applySig(f, out, sig); err != nil {
		return err
	}
	// Only a public symbol leaves the object file. Package linkage is
	// visible to the modules built alongside this one, which is a
	// question for whoever links them and not one an object file can
	// answer, so it is internal here too.
	switch f.Linkage() {
	case vil.Public, vil.PublicExternal:
		out.Export()
	default:
		out.Internal()
	}
	l.callee[f.Name()] = out
	return nil
}

// signature is the VIR calling signature of a lowered VIL function
// type. Conventions do not survive: @owned and @guaranteed say who
// releases the value, and by this point the retains and releases are
// already written down as instructions.
func (l *lowerer) signature(name string, t *vil.FuncType) (*ir.Sig, error) {
	sig := ir.NewSig()
	// A result too wide for registers is not returned at all: the
	// caller sets storage aside and passes its address, and the
	// callee writes through it. That parameter comes first, which is
	// where ir requires an sret and where AAPCS64 puts the register
	// that carries it -- X8, beside the argument sequence rather than
	// in it. It is also where the entry block already looks for it
	// and where the call site already puts it; only this signature
	// said otherwise, and said it about an attribute nothing was
	// carrying.
	for _, res := range t.Results {
		// `@out` says so whatever the size: a generic function's
		// result comes back through the caller's storage because the
		// caller is the only one who knows how big it is. That is
		// ir.SwiftIndirectResult, which is the register by
		// declaration; sret is the register by size, and a result too
		// wide for the return sequence reaches it that way.
		if res.Convention == vil.ResultOut {
			sig.Param(ir.TypePtr, ir.SwiftIndirectResult)
			break
		}
		if _, wide := indirect(res.Type); wide {
			sig.Param(ir.TypePtr, ir.SRet(l.sretType(res.Type)))
			break
		}
	}
	for i, p := range t.Params {
		// Nothing to pass. A parameter of empty type -- Void, the
		// empty tuple, a struct with no stored properties -- takes no
		// register, which is what the entry block already assumes
		// when it skips such an argument.
		if empty(p.Type) {
			continue
		}
		// One too wide for registers is passed as its address: swiftc's
		// own code for a five-word argument reads the fields out of
		// x0 rather than out of x0 to x4.
		if _, yes := indirect(p.Type); yes {
			sig.Param(ir.TypePtr)
			continue
		}
		// A tuple is flattened into the parameter list rather than
		// packed into words: it is the parameter list, and Swift
		// treats it as one. See tupleParts.
		if rs, ok := tupleParts(p.Type); ok {
			for _, r := range rs {
				sig.Param(r.reg)
			}
			continue
		}
		// A struct of more than one field is more than one register:
		// its memory image cut into words, which is what Swift passes.
		if n, ok := directWords(p.Type); ok {
			for i := 0; i < n; i++ {
				sig.Param(ir.TypeI64)
			}
			continue
		}
		r, ok := machine(p.Type)
		if !ok {
			return nil, &Error{Err: ErrType, Func: name, What: whyNoRegister(p.Type)}
		}
		if selfRegister(t, i) {
			sig.Param(r.reg, ir.SwiftSelf)
			continue
		}
		sig.Param(r.reg)
	}
	// A witness carries two more arguments than it declares: the
	// metadata for the type that is answering, and the table the call
	// came through. SIL does not write them down either -- they are
	// what a substitution turns into -- so they are added here rather
	// than in the VIL, and the call site reads them out of the
	// existential it looked the witness up in. swiftc's own code says
	// where: `x.w(k, j)` through `any P2` leaves k in w0 and j in w1
	// and puts the metadata in x2 and the table in x3.
	if t.Convention == vil.ConvWitness {
		sig.Param(ir.TypePtr)
		sig.Param(ir.TypePtr)
	}
	for _, res := range t.Results {
		if empty(res.Type) {
			continue
		}
		if _, yes := indirect(res.Type); yes || res.Convention == vil.ResultOut {
			continue
		}
		if n, ok := directWords(res.Type); ok {
			for i := 0; i < n; i++ {
				sig.Ret(ir.TypeI64)
			}
			continue
		}
		r, ok := machine(res.Type)
		if !ok {
			return nil, &Error{Err: ErrType, Func: name, What: whyNoRegister(res.Type)}
		}
		sig.Ret(r.reg)
	}
	// A function that may fail brings the failure back in the error
	// register rather than in the return sequence: the caller clears
	// it, the callee writes it on the path that fails, and the caller
	// reads it after. See ir.SwiftError.
	if t.ErrorType.IsValid() {
		sig.Ret(ir.TypePtr, ir.SwiftError)
	}
	if t.Async {
		return nil, &Error{Err: ErrUnsupported, Func: name, What: "an async function"}
	}
	return sig, nil
}

// applySig states the signature again through the calls that also
// create the function's parameter registers, which is how ir.Func
// wants it said. ir has one method per reg-type on purpose, so a verb
// it does not have is a compile error rather than a refusal at run
// time; the price is this switch.
func (l *lowerer) applySig(f *vil.Func, out *ir.Func, sig *ir.Sig) error {
	for i, p := range sig.Params() {
		name := "a" + itoa(i)
		// The attributes come with it. They are not decoration: one
		// of them says the parameter travels in the self register
		// rather than in the argument sequence, and a definition that
		// dropped it read self out of x0 while every call to it wrote
		// x20. Both halves were built from this one signature, and
		// only the calls kept what it said.
		attrs := p.Attrs
		switch p.Type {
		case ir.TypeI1:
			out.ParamI1(name, attrs...)
		case ir.TypeI32:
			out.ParamI32(name, attrs...)
		case ir.TypeI64:
			out.ParamI64(name, attrs...)
		case ir.TypeF32:
			out.ParamF32(name, attrs...)
		case ir.TypeF64:
			out.ParamF64(name, attrs...)
		case ir.TypePtr:
			out.ParamPtr(name, attrs...)
		default:
			return &Error{Err: ErrType, Func: f.Name(), What: p.Type.String()}
		}
	}
	for _, r := range sig.Rets() {
		switch r.Type {
		case ir.TypeI1:
			out.ReturnsI1()
		case ir.TypeI32:
			out.ReturnsI32()
		case ir.TypeI64:
			out.ReturnsI64()
		case ir.TypeF32:
			out.ReturnsF32()
		case ir.TypeF64:
			out.ReturnsF64()
		case ir.TypePtr:
			out.ReturnsPtr()
		default:
			return &Error{Err: ErrType, Func: f.Name(), What: r.Type.String()}
		}
	}
	return nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// funcTypeOf is the VIR func typedef for a Swift function type, made
// once per distinct type and reused.
//
// The name is the type's own spelling with what VIR's identifiers do
// not admit taken out. It has to be a name because a callind names a
// typedef rather than carrying a signature inline; it has to be one
// name per signature because two typedefs with the same shape are two
// types.
func (l *lowerer) funcTypeOf(sig *types.Signature) (*ir.Type, error) {
	name := funcTypeName(sig)
	if t, ok := l.funcTypes[name]; ok {
		return t, nil
	}
	s := ir.NewSig()
	for _, p := range sig.Params {
		if empty(vil.Object(p.Type)) {
			continue
		}
		if n, ok := directWords(vil.Object(p.Type)); ok {
			for i := 0; i < n; i++ {
				s.Param(ir.TypeI64)
			}
			continue
		}
		r, ok := machineOf(p.Type)
		if !ok {
			return nil, fmt.Errorf("no register for %s", p.Type)
		}
		s.Param(r.reg)
	}
	if sig.Results != nil && !isVoidType(sig.Results) {
		if n, ok := directWords(vil.Object(sig.Results)); ok {
			for i := 0; i < n; i++ {
				s.Ret(ir.TypeI64)
			}
		} else {
			r, ok := machineOf(sig.Results)
			if !ok {
				return nil, fmt.Errorf("no register for %s", sig.Results)
			}
			s.Ret(r.reg)
		}
	}
	t := l.out.FuncType(name, s)
	if l.funcTypes == nil {
		l.funcTypes = map[string]*ir.Type{}
	}
	l.funcTypes[name] = t
	return t, nil
}

// funcTypeName is a VIR identifier standing for a Swift function type.
func funcTypeName(sig *types.Signature) string {
	var b strings.Builder
	b.WriteString("fn")
	for _, p := range sig.Params {
		b.WriteByte('_')
		b.WriteString(identSafe(p.Type.String()))
	}
	b.WriteString("__")
	if sig.Results != nil && !isVoidType(sig.Results) {
		b.WriteString(identSafe(sig.Results.String()))
	} else {
		b.WriteString("void")
	}
	return b.String()
}

// identSafe turns a type's spelling into something VIR will take as an
// identifier: letters, digits and underscore, never starting with a
// digit.
//
// A hash of the original goes on the end because the substitution is
// not injective -- `(Int, Int)` and `[Int: Int]` flatten to the same
// letters -- and two distinct signatures sharing one typedef would be
// two different calls agreeing to disagree about their arguments.
func identSafe(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	// FNV-1a, for a short distinct tail.
	var h uint64 = 14695981039346656037
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= 1099511628211
	}
	return b.String() + "_" + strconv.FormatUint(h, 16)
}

// isVoidType is the empty result, which VIR spells by having none.
func isVoidType(t types.Type) bool {
	b, ok := t.Underlying().(*types.Basic)
	return ok && (b.Kind() == types.Void)
}

// vtables lays each class's dispatch table down as a read-only global
// of function pointers.
//
// One pointer per row, in the order gen wrote them, which is the order
// the chain declares them: an inherited slot keeps the index it had in
// the superclass, so a call that loaded a table it has never seen
// still indexes it correctly. That invariant is the whole of dynamic
// dispatch and it is gen that establishes it -- see vil/gen/vtable.go.
//
// Read-only because a table is written once, by the linker, out of
// relocations to the functions it names.
func (l *lowerer) vtables(m *vil.Module) error {
	tables := m.VTables()
	if len(tables) == 0 {
		return nil
	}
	l.vtable = make(map[string]*ir.Global, len(tables))
	l.slots = make(map[string][]string, len(tables))
	for _, t := range tables {
		if len(t.Entries) == 0 {
			// A class with no methods has nothing to dispatch, and an
			// empty array is not a type VIR admits.
			continue
		}
		rows := make([]ir.Init, 0, len(t.Entries))
		members := make([]string, 0, len(t.Entries))
		for _, e := range t.Entries {
			impl, ok := l.callee[l.sym(e.Impl)]
			if !ok {
				f, ok := l.defs[e.Impl]
				if !ok {
					return &Error{Err: ErrUnsupported, Func: t.Class,
						What: "no function named " + e.Impl + " for slot " + e.Member}
				}
				impl = f
			}
			rows = append(rows, ir.RelocInit(impl))
			members = append(members, e.Member)
		}
		g := l.out.Global(l.sym(vtableSymbol(t.Class)), ir.RO,
			ir.Array(uint64(len(rows)), ir.StorePtr.FType())).Init(ir.List(rows...))
		l.vtable[t.Class] = g
		l.slots[t.Class] = members
	}
	return nil
}

// vtableSymbol is the object-file name of a class's table.
func vtableSymbol(class string) string { return "$sv" + class }

// lowerFuncType is the VIR typedef for an already-lowered VIL function
// type, which is what a class_method yields.
func (l *lowerer) lowerFuncType(t *vil.FuncType) (*ir.Type, error) {
	name := "fnv_" + identSafe(t.String())
	if got, ok := l.funcTypes[name]; ok {
		return got, nil
	}
	sig, err := l.signature(name, t)
	if err != nil {
		return nil, err
	}
	ft := l.out.FuncType(name, sig)
	if l.funcTypes == nil {
		l.funcTypes = map[string]*ir.Type{}
	}
	l.funcTypes[name] = ft
	return ft, nil
}

// sretType is the shape of the storage an indirect result is written
// into, made once per result type and reused.
//
// A struct of words rather than of the result's own fields: what the
// backend reads from it is how big it is and whether it is a
// homogeneous float aggregate, and a struct wide enough to be
// returned this way is neither small nor homogeneous. Naming it after
// the type keeps two different results from sharing one shape.
func (l *lowerer) sretType(t vil.Type) *ir.Type {
	size, ok := indirect(t)
	if !ok {
		// A result that is `@out` rather than merely wide. Its size
		// is the type's, which is what the caller has to set aside.
		size, _ = storageBytes(t)
	}
	name := "sret_" + identSafe(t.String())
	if got, ok := l.sretTypes[name]; ok {
		return got
	}
	out := l.out.Struct(name)
	for i := int64(0); i < (size+7)/8; i++ {
		out.Field("w"+strconv.FormatInt(i, 10), ir.StoreI64.FType())
	}
	if l.sretTypes == nil {
		l.sretTypes = map[string]*ir.Type{}
	}
	l.sretTypes[name] = out
	return out
}

// witnessTables lays each conformance down as a read-only table of
// the functions that satisfy it.
//
// A table is the conformance descriptor, then one row for each
// protocol this one inherits holding that conformance's own table,
// then one row per requirement in the order the protocol declares
// them. Every table for a protocol shares that order, because a call
// through an existential knows which protocol it is calling and not
// which type is answering -- which is the whole difference from a
// vtable, where the class is known and the order comes from the
// chain.
//
// The order is swiftc's, read off its own output: `Sub: Base` puts
// Base's table in the row after the descriptor and Sub's own
// requirements after that, so `x.b()` through `any Sub` is two loads
// and `x.s()` is one.
func (l *lowerer) witnessTables(m *vil.Module) error {
	// The order comes from the protocol rather than from any table:
	// a call through an existential whose protocol is imported has no
	// table here to read it from. See vil.Module.Requirements.
	l.witnessRows = make(map[string][]string, len(m.Protocols()))
	for proto, names := range m.Protocols() {
		l.witnessRows[proto] = names
	}
	tables := m.WitnessTables()
	if len(tables) == 0 {
		return nil
	}
	// Every table gets its symbol before any is filled in: a table
	// for a protocol that inherits another holds that one's table in
	// a row, and the two are emitted in whatever order the source put
	// them in.
	l.witness = make(map[string]*ir.Global, len(tables))
	for _, t := range tables {
		if t == nil || len(t.Entries) == 0 {
			continue
		}
		order, ok := l.witnessRows[t.Protocol]
		if !ok {
			return &Error{Err: ErrIR, Func: t.Type,
				What: "a conformance to " + t.Protocol +
					" whose protocol did not say what order its requirements are in"}
		}
		g := l.out.Global(l.sym(witnessSymbol(t)), ir.RO,
			ir.Array(uint64(len(order)+1), ir.StorePtr.FType()))
		g.Export()
		l.witness[t.Type+":"+t.Protocol] = g
	}
	for _, t := range tables {
		if t == nil || len(t.Entries) == 0 {
			continue
		}
		order := l.witnessRows[t.Protocol]
		rows := make([]ir.Init, 0, len(order)+1)
		// The first word is the conformance descriptor, which is what
		// makes a table something a reader can ask questions of
		// rather than a bare row list. A conformance to a protocol
		// this module declared has none: there is no protocol
		// descriptor here for it to point at.
		if d, ok := l.conformanceDescriptor(t, l.witness[t.Type+":"+t.Protocol]); ok {
			rows = append(rows, ir.RelocInit(d))
		} else {
			rows = append(rows, ir.Lit(ir.Int(0)))
		}
		for _, want := range order {
			e, ok := entryFor(t.Entries, want)
			if !ok {
				return &Error{Err: ErrUnsupported, Func: t.Type,
					What: "no witness for " + want + " in the conformance to " + t.Protocol}
			}
			// A row naming another table rather than a function: the
			// same type's conformance to the protocol this one
			// inherits.
			if base, ok := baseOf(want); ok {
				g, ok := l.witness[t.Type+":"+base]
				if !ok {
					return &Error{Err: ErrUnsupported, Func: t.Type,
						What: "no conformance to " + base + ", which " + t.Protocol + " inherits"}
				}
				rows = append(rows, ir.RelocInit(g))
				continue
			}
			impl, ok := l.callee[l.sym(e.Impl)]
			if !ok {
				f, ok := l.defs[e.Impl]
				if !ok {
					return &Error{Err: ErrUnsupported, Func: t.Type,
						What: "no function named " + e.Impl + " for " + e.Member}
				}
				impl = f
			}
			rows = append(rows, ir.RelocInit(impl))
		}
		l.witness[t.Type+":"+t.Protocol].Init(ir.List(rows...))
	}
	return nil
}

// baseOf takes a row apart where it names an inherited conformance's
// table rather than a function: `Labelled:Sized` is one and
// `Labelled.size` is not. See baseRow in vil/gen.
func baseOf(row string) (string, bool) {
	i := strings.IndexByte(row, ':')
	if i < 0 {
		return "", false
	}
	return row[i+1:], true
}

// entryFor finds the row that satisfies one requirement.
func entryFor(entries []vil.TableEntry, member string) (vil.TableEntry, bool) {
	for _, e := range entries {
		if e.Member == member {
			return e, true
		}
	}
	return vil.TableEntry{}, false
}

// witnessSymbol is the object-file name of a conformance's table,
// which is swiftc's own: `$s3App4MineV3Lib8MeasuredAAWP`. The name is
// mangled where the conformance is read -- see conformanceSymbols in
// vil/gen -- and a conformance this compiler could not name keeps a
// private one, which links inside this module and nowhere else.
func witnessSymbol(t *vil.WitnessTable) string {
	if t.Symbol != "" {
		return t.Symbol
	}
	return "$sWT" + identSafe(t.Type) + "_" + identSafe(t.Protocol)
}

// selfRegister reports whether a parameter travels in the self
// register rather than in the argument sequence.
//
// Swift's rule is about what the receiver is rather than about the
// method: a class's method takes self in X20, and a value type's
// takes it as its last ordinary argument. swiftc's own code says both
// -- `C.plus(_ k: Int32)` reads k from w0 and self from x20, while
// `S.plus(_ k: Int32)` reads k from w0 and self.a from w1.
//
// Getting it wrong is not a crash. The callee reads whatever was in
// X20 and returns a number, so a call across a module boundary
// compiles, links and answers: 113 where swiftc answered 42.
//
// Only the receiver of a method, and only where it is a class
// reference. What a witness's self does, and what a mutating method's
// inout self does, are two more questions this does not answer yet --
// both stay inside this compiler today, where the two ends agree
// because they are the same end.
func selfRegister(t *vil.FuncType, i int) bool {
	// A witness takes self in the self register whatever the type is:
	// swiftc's thunk for `A.v()` is `ldr w0, [x20]`, and A is a
	// struct. It arrives as an address, which is the other half of
	// the witness convention -- the caller holds an existential and
	// not a value of the type.
	if t.Convention == vil.ConvWitness {
		return i == len(t.Params)-1
	}
	if t.Convention != vil.Method || i != len(t.Params)-1 {
		return false
	}
	switch f := t.Params[i].Type.Formal().(type) {
	case *vil.MetatypeType:
		// An allocating initializer's self is the metatype, and a
		// class's is a real pointer that travels in the self register
		// like any other receiver. A struct's is thin -- no bits, no
		// register -- and never reaches here.
		return !f.Thin()
	}
	_, isClass := t.Params[i].Type.Formal().Underlying().(*types.Class)
	return isClass
}

// destroyWitnessType is the shape of a destroy value witness: the
// value's address and the metadata for its type, and no result.
func (l *lowerer) destroyWitnessType() *ir.Type {
	if l.destroyWitness == nil {
		l.destroyWitness = l.out.FuncType("vw_destroy",
			ir.NewSig().Param(ir.TypePtr).Param(ir.TypePtr))
	}
	return l.destroyWitness
}

// metadataAccessor is the function a class's metadata comes from,
// declared once per name.
//
// Its shape is fixed by Swift and not by anything here: it takes a
// metadata request -- zero for the complete metadata, which is what a
// client asks for -- and returns the metadata pointer.
func (l *lowerer) metadataAccessor(sym string) ir.Callee {
	if l.accessors == nil {
		l.accessors = map[string]ir.Callee{}
	}
	if got, ok := l.accessors[sym]; ok {
		return got
	}
	fn := l.out.ImportFunc(l.prefix+sym, ir.NewSig().Param(ir.TypeI64).Ret(ir.TypePtr))
	l.accessors[sym] = fn
	return fn
}
