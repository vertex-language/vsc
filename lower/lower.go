package lower

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/vertex-language/ir"
	"github.com/vertex-language/vsc/internal/sil"
	"github.com/vertex-language/vsc/stdlib"
	"github.com/vertex-language/vsc/types"
)

// Options are what the caller decides about the translation.
type Options struct {
	// SymbolPrefix is prepended to object file symbols ("_" on Mach-O, empty on ELF/COFF).
	SymbolPrefix string
	// Kernels is this module's kernels, by SIL name, with what the
	// device compile built for each: each gets a descriptor and a CPU
	// thunk beside it. See gpu.go.
	Kernels map[string]*KernelImage
	// Device lowers for a GPU: the gpu intrinsics as the VIR verbs they
	// name, and an entry point for each of DeviceKernels.
	Device        bool
	DeviceKernels []string
}

// Module translates a lowered SIL module (sil.StageLowered) into a VIR module for the target.
func Module(m *sil.Module, target ir.Target, opts Options) (*ir.Module, error) {
	if m.Stage() != sil.StageLowered {
		return nil, &Error{Err: ErrStage, What: string(m.Stage())}
	}
	l := &lowerer{
		out:    ir.NewModule(m.Name(), target),
		module: m,
		prefix: opts.SymbolPrefix,
		callee: make(map[string]ir.Callee),
		defs:   make(map[string]*ir.Func),
	}
	l.ms = l.out.Layout().ABI == "ms"
	l.kernels = opts.Kernels
	l.device = opts.Device
	l.deviceKernels = opts.DeviceKernels
	// How big a context each async function wants, before anything is
	// lowered: a caller allocates the callee's, so it has to know.
	l.asyncSizes = map[string]int64{}
	l.plans = map[string]*asyncPlan{}
	for _, f := range m.Funcs() {
		p, err := planAsync(l, f)
		if err != nil {
			return nil, err
		}
		if p == nil {
			continue
		}
		l.plans[f.Name()] = p
		l.asyncSizes[f.Name()] = p.size
	}
	for _, f := range m.Funcs() {
		if err := l.declare(f); err != nil {
			return nil, err
		}
	}
	// The record beside each async function this module defines, so
	// that a caller anywhere can size the frame it has to allocate.
	// See asyncsplit.go.
	for _, f := range m.Funcs() {
		if size, ok := l.asyncSizes[f.Name()]; ok {
			l.asyncRecord(f.Name(), size)
		}
	}
	if err := l.vtables(m); err != nil {
		return nil, err
	}
	// The protocols' descriptors first: an existential's metadata names
	// them. Then metadata and tables, before function bodies.
	l.protocolDescriptors(m)
	if err := l.allMetadata(m); err != nil {
		return nil, err
	}
	if err := l.witnessTables(m); err != nil {
		return nil, err
	}
	for _, f := range m.Funcs() {
		if l.declaredHere(f) {
			continue
		}
		if err := l.define(f); err != nil {
			return nil, err
		}
	}
	// Each kernel's descriptor, whether or not this module launches it:
	// another module may. For a device, each kernel's entry.
	if l.device {
		if err := l.kernelEntries(); err != nil {
			return nil, err
		}
	} else {
		for _, f := range m.Funcs() {
			if _, ok := l.kernels[f.Name()]; ok {
				l.kernelDescriptor(f.Name())
			}
		}
	}
	l.weakenCoreRecords()
	if err := l.out.Err(); err != nil {
		return nil, &Error{Err: ErrIR, What: err.Error()}
	}
	return l.out, nil
}

// weakenCoreRecords makes every exported definition of a core type's --
// its metadata, descriptor, value witnesses and accessor, mangled in the
// standard library's module ($sS…, $ss…) -- a weak one. Core is compiled
// into each module that uses it, so two modules both needing Character's
// metadata both define it, alike, and the linker keeps one, as it keeps one
// copy of a C++ inline function.
func (l *lowerer) weakenCoreRecords() {
	core := func(name string) bool {
		return strings.HasPrefix(name, l.sym("$sS")) || strings.HasPrefix(name, l.sym("$ss"))
	}
	for _, g := range l.out.Globals() {
		if g.Linkage() == ir.Export && core(g.Name()) {
			g.Weak()
		}
	}
	for _, f := range l.out.Funcs() {
		if f.Linkage() == ir.Export && core(f.Name()) {
			f.Weak()
		}
	}
}

// lowerer maintains translation state across a lowered module.
type lowerer struct {
	emptyTable  *ir.Global               // emptyWitnessTable's, once made
	closures    map[string]closureParts  // capturing closures' forwarders, by body
	releasers   map[string]*ir.Func      // a stack context's last release, by body; nil for none
	onces       map[string]*onceAccessor // global addressors initialized once, by name; nil for not one
	boxes       map[string]*ir.Global    // captured variables' box metadata, by element type
	globalStore map[string]*ir.Global    // module-level variables, by symbol
	out         *ir.Module
	prefix      string
	// The gpu's: this module's kernels and what was built for them,
	// whether this is a device's lowering and of which kernels, and the
	// descriptors made or imported. See gpu.go.
	kernels       map[string]*KernelImage
	device        bool
	deviceKernels []string
	kernelDescs   map[string]ir.Symbol
	kernelRecord  *ir.Type
	callee        map[string]ir.Callee
	defs          map[string]*ir.Func

	witness     map[string]*ir.Global
	witnessRows map[string][]string

	vtable map[string]*ir.Global
	slots  map[string][]string

	funcTypes map[string]*ir.Type
	sretTypes map[string]*ir.Type
	// asyncRecords and asyncImports are the records beside async
	// functions: what this module places, and what it reads from
	// another. See asyncsplit.go.
	asyncRecords map[string]*ir.Global
	asyncImports map[string]*ir.GlobalImport
	// plans is how each async function in this module is split, worked
	// out before any of it is lowered. See async.go.
	plans map[string]*asyncPlan
	// asyncSizes is how big a context each async function in this module
	// wants, worked out before anything is lowered because a caller
	// allocates the callee's. See asyncFrameSize.
	asyncSizes map[string]int64
	// exported is each function that leaves the object file, by SIL
	// name: what its async record does too (see asyncRecord).
	exported map[string]bool

	meta        map[string]*ir.Global
	vwts        map[string]*ir.Global
	descriptors map[string]*ir.Global
	// importedDescriptors is the protocol descriptors other modules define.
	importedDescriptors map[string]*ir.GlobalImport
	strings2            map[string]*ir.Global
	witnessFns          map[string]*ir.Func
	records             map[string]*ir.GlobalImport

	module         *sil.Module
	destroyWitness *ir.Type
	copyWitness    *ir.Type

	strings  map[string]*ir.Global
	literals int

	bridgeRetain  ir.Callee
	bridgeRelease ir.Callee

	runtimeFns map[string]ir.Callee
	ms         bool
	accessors  map[string]ir.Callee

	retain  ir.Callee
	release ir.Callee
	// existentialRetain and existentialRelease count an existential held
	// as a value. See countExistentialWords.
	existentialRetain, existentialRelease ir.Callee
	alloc                                 ir.Callee
}

// declaredHere reports whether f is only declared in this module's
// output: it has no body, or it is another module's @inlinable function
// (public_external with a body), which host code calls in the module
// that defines it. A device compile has no other module to call into,
// so there the body is the definition.
func (l *lowerer) declaredHere(f *sil.Func) bool {
	if f.IsDeclaration() {
		return true
	}
	return !l.device && f.Linkage() == sil.PublicExternal
}

// sym prepends the platform symbol prefix to a SIL identifier.
func (l *lowerer) sym(name string) string { return l.prefix + name }

// declare gives every SIL function a VIR symbol before any body is
// filled, so a call to a function defined later has something to name.
func (l *lowerer) declare(f *sil.Func) error {
	sig, err := l.signature(f.Name(), f.Type())
	if err != nil {
		return err
	}
	if l.declaredHere(f) {
		l.callee[f.Name()] = l.out.ImportFunc(l.sym(f.Name()), sig)
		return nil
	}
	out := l.out.Func(l.sym(f.Name()))
	l.defs[f.Name()] = out
	if err := l.applySig(f, out, sig); err != nil {
		return err
	}
	// A public symbol leaves the object file, and so does a module's
	// internal one (hidden, package): a program is every module it imports
	// linked into one image, and another module's specialization of this
	// one's generic code calls this one's internal functions, as Swift's
	// @usableFromInline makes them callable. Names carry their module, so
	// two modules' internals cannot collide. Only private stays in the
	// object: closures, and each module's own copy of a specialization.
	switch f.Linkage() {
	case sil.Public, sil.PublicExternal, sil.Hidden, sil.PackageLinkage:
		out.Export()
		if l.exported == nil {
			l.exported = map[string]bool{}
		}
		l.exported[f.Name()] = true
	default:
		out.Internal()
	}
	l.callee[f.Name()] = out
	return nil
}

// signature is the VIR calling signature of a lowered SIL function
// type. Conventions do not survive: @owned and @guaranteed say who
// releases the value, and by this point the retains and releases are
// already written down as instructions.
func (l *lowerer) signature(name string, t *sil.FuncType) (*ir.Sig, error) {
	sig := ir.NewSig()
	async := t.Async
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
		if res.Convention == sil.ResultOut {
			sig.Param(ir.TypePtr, ir.SwiftIndirectResult)
			break
		}
		if _, wide := indirect(res.Type); wide {
			sig.Param(ir.TypePtr, ir.SRet(l.sretType(res.Type)))
			break
		}
		if _, split := l.splitResult(res.Type); split {
			sig.Param(ir.TypePtr, ir.SRet(l.sretType(res.Type)))
			break
		}
	}
	// Then the context, for an async function, which returns nothing:
	// its result goes to the continuation the context names, which is
	// how a function that may be resumed on another thread gets its
	// answer home. After the storage rather than before it, which is
	// the order swiftc writes:
	//
	//	define swifttailcc void @make(ptr sret %0, ptr swiftasync %1, i64 %2)
	//
	// Neither takes a register out of the argument sequence -- an
	// indirect result travels in X8 and a context in X22 -- so the
	// order is bookkeeping, and it is Swift's bookkeeping.
	// See stdlib's AsyncContext.
	if async {
		sig.Param(ir.TypePtr, ir.SwiftAsync)
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
	// than in the SIL, and the call site reads them out of the
	// existential it looked the witness up in. swiftc's own code says
	// where: `x.w(k, j)` through `any P2` leaves k in w0 and j in w1
	// and puts the metadata in x2 and the table in x3.
	if t.Convention == sil.ConvWitness {
		sig.Param(ir.TypePtr)
		sig.Param(ir.TypePtr)
	}
	for _, res := range t.Results {
		if empty(res.Type) {
			continue
		}
		// An async function hands its results forward rather than back,
		// so it declares none. They become the continuation's arguments.
		if async {
			continue
		}
		if _, yes := indirect(res.Type); yes || res.Convention == sil.ResultOut {
			continue
		}
		if _, split := l.splitResult(res.Type); split {
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
	//
	// An async one declares no result for it either, because it
	// declares no results at all. Its failure goes forward with
	// everything else: the continuation takes the error as a last
	// argument in the self register, null where nothing was thrown.
	// That is measured from swiftc, which ends a throwing async
	// function with
	//
	//	musttail call swifttailcc void %resume(ptr swiftasync %ctx,
	//	                                       i64 %result, ptr swiftself %err)
	//
	// and on the failing path passes undef for the result. See
	// docs/vertex_swift_async.md.
	if t.ErrorType.IsValid() && !async {
		sig.Ret(ir.TypePtr, ir.SwiftError)
	}
	return sig, nil
}

// applySig states the signature again through the calls that also
// create the function's parameter registers, which is how ir.Func
// wants it said. ir has one method per reg-type on purpose, so a verb
// it does not have is a compile error rather than a refusal at run
// time; the price is this switch.
func (l *lowerer) applySig(f *sil.Func, out *ir.Func, sig *ir.Sig) error {
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
		case ir.TypeF16:
			out.ParamF16(name, attrs...)
		case ir.TypeBF16:
			out.ParamBF16(name, attrs...)
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
			out.ReturnsI1(r.Attrs...)
		case ir.TypeI32:
			out.ReturnsI32(r.Attrs...)
		case ir.TypeI64:
			out.ReturnsI64(r.Attrs...)
		case ir.TypeF32:
			out.ReturnsF32(r.Attrs...)
		case ir.TypeF16:
			out.ReturnsF16(r.Attrs...)
		case ir.TypeBF16:
			out.ReturnsBF16(r.Attrs...)
		case ir.TypeF64:
			out.ReturnsF64(r.Attrs...)
		case ir.TypePtr:
			out.ReturnsPtr(r.Attrs...)
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
// continuationType is the type of the function an async return tail calls:
// the context, then the results as ordinary arguments, returning nothing.
//
// Named by its argument shape, since every async function whose results
// are the same registers returns through the same kind of continuation.
func (l *lowerer) continuationType(results []ir.Value, throws bool) *ir.Type {
	sig := ir.NewSig().Param(ir.TypePtr, ir.SwiftAsync)
	name := "async_cont"
	for _, r := range results {
		t := r.RegType()
		sig.Param(t)
		name += "_" + t.String()
	}
	// The error last, in the self register, as swiftc passes it.
	if throws {
		sig.Param(ir.TypePtr, ir.SwiftSelf)
		name += "_err"
	}
	if t, ok := l.funcTypes[name]; ok {
		return t
	}
	t := l.out.FuncType(name, sig)
	if l.funcTypes == nil {
		l.funcTypes = map[string]*ir.Type{}
	}
	l.funcTypes[name] = t
	return t
}

func (l *lowerer) funcTypeOf(sig *types.Signature) (*ir.Type, error) {
	name := funcTypeName(sig)
	if t, ok := l.funcTypes[name]; ok {
		return t, nil
	}
	s, err := l.funcSig(sig)
	if err != nil {
		return nil, err
	}
	t := l.out.FuncType(name, s)
	if l.funcTypes == nil {
		l.funcTypes = map[string]*ir.Type{}
	}
	l.funcTypes[name] = t
	return t, nil
}

// funcSig is the VIR signature a call through a function value of this
// Swift type uses: its arguments, its context in the self register, its
// results, and the error register for one that throws.
func (l *lowerer) funcSig(sig *types.Signature) (*ir.Sig, error) {
	s := ir.NewSig()
	// A result too wide for registers is returned through storage the
	// caller sets aside, whose address is the first parameter -- for an
	// async function as much as a synchronous one, since the async ABI
	// passes that pointer as a parameter and only a register-width result
	// travels to the continuation. This mirrors lowerer.signature, the
	// callee side, and the call site in asyncsplit, which prepends the
	// same pointer (see answerStorage). Leaving it out here was a bug
	// that only showed once a result grew past the register threshold.
	if sig.Results != nil && !isVoidType(sig.Results) {
		if _, wide := indirect(sil.Object(sig.Results)); wide {
			s.Param(ir.TypePtr, ir.SRet(l.sretType(sil.Object(sig.Results))))
		} else if _, split := l.splitResult(sil.Object(sig.Results)); split {
			s.Param(ir.TypePtr, ir.SRet(l.sretType(sil.Object(sig.Results))))
		}
	}
	// An async function value is called the way an async function is:
	// the context first, and no register results, because a
	// register-width answer goes to the continuation that context names.
	// See lowerer.signature, which says the same thing about a declared
	// one.
	if sig.Async {
		s.Param(ir.TypePtr, ir.SwiftAsync)
	}
	for _, p := range sig.Params {
		// An inout parameter is the address of the caller's storage,
		// whatever it holds, as the closure it calls takes it.
		if p.Ownership == types.InOut {
			s.Param(ir.TypePtr)
			continue
		}
		if empty(sil.Object(p.Type)) {
			continue
		}
		// One too wide for registers goes by address, as it does in a
		// direct call: see lowerer.signature, and the call site in
		// inst.go, which is the same code for both.
		if _, wide := indirect(sil.Object(p.Type)); wide {
			s.Param(ir.TypePtr)
			continue
		}
		// A tuple goes as its elements, each in its own register, as a
		// direct call's parameter does (lowerer.signature) and as the call
		// site gathers it (gatherArg): an (Int, Double) is an i64 and an f64.
		if rs, ok := tupleParts(sil.Object(p.Type)); ok {
			for _, r := range rs {
				s.Param(r.reg)
			}
			continue
		}
		if n, ok := directWords(sil.Object(p.Type)); ok {
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
	if sig.Results != nil && !isVoidType(sig.Results) && !sig.Async {
		if _, wide := indirect(sil.Object(sig.Results)); wide {
			// Already declared as the leading sret pointer above.
		} else if _, split := l.splitResult(sil.Object(sig.Results)); split {
			// Likewise.
		} else if n, ok := directWords(sil.Object(sig.Results)); ok {
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
	// Its context, last, in the self register: null for a function that
	// captures nothing, which never reads the register.
	s.Param(ir.TypePtr, ir.SwiftSelf)
	// A function value that may fail hands its error back in the error
	// register, as a direct call to one does. See ir.SwiftError. An
	// async one hands it forward instead, to the continuation, so it
	// declares no result for it either.
	if sig.Throws && !sig.Async {
		s.Ret(ir.TypePtr, ir.SwiftError)
	}
	return s, nil
}

// funcTypeName is a VIR identifier standing for a Swift function type.
func funcTypeName(sig *types.Signature) string {
	var b strings.Builder
	b.WriteString("fn")
	for _, p := range sig.Params {
		b.WriteByte('_')
		if p.Ownership == types.InOut {
			b.WriteString("inout_")
		}
		b.WriteString(identSafe(p.Type.String()))
	}
	b.WriteString("__")
	if sig.Results != nil && !isVoidType(sig.Results) {
		b.WriteString(identSafe(sig.Results.String()))
	} else {
		b.WriteString("void")
	}
	if sig.Throws {
		// A different shape: one more result, in the error register.
		b.WriteString("_throws")
	}
	if sig.Async {
		// A different shape again: a context first and no results.
		b.WriteString("_async")
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

// vtables emits read-only global function pointer tables for class dynamic dispatch.
func (l *lowerer) vtables(m *sil.Module) error {
	tables := m.VTables()
	if len(tables) == 0 {
		return nil
	}
	if l.vtable == nil {
		l.vtable = make(map[string]*ir.Global, len(tables))
	}
	l.slots = make(map[string][]string, len(tables))
	for _, t := range tables {
		destroy := l.classDestroyer(t)
		// Layout: [destroyer, class metadata, method pointers...].
		rows := make([]ir.Init, 0, len(t.Entries)+vtableFirstMethod)
		if destroy != nil {
			rows = append(rows, ir.RelocInit(destroy))
		} else {
			rows = append(rows, ir.Lit(ir.Int(0)))
		}
		if record, ok := l.classMetadataFor(t.Class); ok {
			rows = append(rows, ir.RelocInit(record).Plus(ir.Int(stdlib.MetadataOffset)))
		} else {
			rows = append(rows, ir.Lit(ir.Int(0)))
		}
		members := make([]string, 0, len(t.Entries))
		for _, e := range t.Entries {
			// An async witness is reached the way any async function value
			// is: through the record beside its code, which says where the
			// code is and how big a context it wants. A call through the
			// table reads the row as that record (see suspensionTarget).
			if _, async := l.asyncSizes[e.Impl]; async {
				rows = append(rows, ir.RelocInit(l.asyncRecordOf(e.Impl)))
				continue
			}
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
		g := l.vtableGlobal(t)
		g.Init(ir.List(rows...))
		l.slots[t.Class] = members
	}
	return nil
}

// vtableGlobal is a class's dispatch table, declared the first time it is
// asked for -- by the class's metadata, which points at it, or by vtables,
// which fills it in.
func (l *lowerer) vtableGlobal(t *sil.VTable) *ir.Global {
	if g, ok := l.vtable[t.Class]; ok {
		return g
	}
	if l.vtable == nil {
		l.vtable = map[string]*ir.Global{}
	}
	g := l.out.Global(l.sym(vtableSymbol(t.Class)), ir.RO,
		ir.Array(uint64(len(t.Entries)+vtableFirstMethod), ir.StorePtr.FType()))
	l.vtable[t.Class] = g
	return g
}

// vtableFirstMethod is the row of a class's table its first method is
// in: row zero is the runtime's destroy slot and row one the class's
// metadata. See stdlib/ABI.md.
const vtableFirstMethod = 2

// vtableSymbol is the object-file name of a class's table.
func vtableSymbol(class string) string { return "$sv" + identSafe(class) }

// lowerFuncType is the VIR typedef for an already-lowered SIL function
// type, which is what a class_method yields.
func (l *lowerer) lowerFuncType(t *sil.FuncType) (*ir.Type, error) {
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

// sretType returns a word-array aggregate type for an indirect return slot.
func (l *lowerer) sretType(t sil.Type) *ir.Type {
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

// witnessTables emits read-only global tables for protocol conformances.
// Layout: [conformance descriptor, inherited conformance tables..., requirement implementations...].
func (l *lowerer) witnessTables(m *sil.Module) error {
	// The order comes from the protocol rather than from any table:
	// a call through an existential whose protocol is imported has no
	// table here to read it from. See sil.Module.Requirements.
	l.witnessRows = make(map[string][]string, len(m.Protocols()))
	for proto, names := range m.Protocols() {
		l.witnessRows[proto] = names
	}
	tables := m.WitnessTables()
	if len(tables) == 0 {
		return nil
	}
	// A conformance to a protocol with no requirements -- Error -- still has
	// a table: the descriptor row alone, which an existential of it holds.
	// Every table gets its symbol before any is filled in: a table
	// for a protocol that inherits another holds that one's table in
	// a row, and the two are emitted in whatever order the source put
	// them in.
	l.witness = make(map[string]*ir.Global, len(tables))
	for _, t := range tables {
		if t == nil {
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
		if t == nil {
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
			// An async witness is reached the way any async function value
			// is: through the record beside its code, which says where the
			// code is and how big a context it wants. A call through the
			// table reads the row as that record (see suspensionTarget).
			if _, async := l.asyncSizes[e.Impl]; async {
				rows = append(rows, ir.RelocInit(l.asyncRecordOf(e.Impl)))
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
// `Labelled.size` is not. See baseRow in sil/gen.
func baseOf(row string) (string, bool) {
	i := strings.IndexByte(row, ':')
	if i < 0 {
		return "", false
	}
	return row[i+1:], true
}

// entryFor finds the row that satisfies one requirement.
func entryFor(entries []sil.TableEntry, member string) (sil.TableEntry, bool) {
	for _, e := range entries {
		if e.Member == member {
			return e, true
		}
	}
	return sil.TableEntry{}, false
}

// witnessSymbol is the object-file name of a conformance's table,
// which is swiftc's own: `$s3App4MineV3Lib8MeasuredAAWP`. The name is
// mangled where the conformance is read -- see conformanceSymbols in
// sil/gen -- and a conformance this compiler could not name keeps a
// private one, which links inside this module and nowhere else.
func witnessSymbol(t *sil.WitnessTable) string {
	if t.Symbol != "" {
		return t.Symbol
	}
	return "$sWT" + identSafe(t.Type) + "_" + identSafe(t.Protocol)
}

// selfRegister reports whether a parameter is passed in the dedicated self register (e.g. X20 on ARM64).
func selfRegister(t *sil.FuncType, i int) bool {
	// A witness takes self in the self register whatever the type is:
	// swiftc's thunk for `A.v()` is `ldr w0, [x20]`, and A is a
	// struct. It arrives as an address, which is the other half of
	// the witness convention -- the caller holds an existential and
	// not a value of the type.
	if t.Convention == sil.ConvWitness {
		return i == len(t.Params)-1
	}
	if t.Convention != sil.Method || i != len(t.Params)-1 {
		return false
	}
	switch f := t.Params[i].Type.Formal().(type) {
	case *sil.MetatypeType:
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

// copyWitnessType is the type of a value witness that copies: destination,
// source, metadata, and the destination back.
func (l *lowerer) copyWitnessType() *ir.Type {
	if l.copyWitness == nil {
		l.copyWitness = l.out.FuncType("vw_copy",
			ir.NewSig().Param(ir.TypePtr).Param(ir.TypePtr).Param(ir.TypePtr).Ret(ir.TypePtr))
	}
	return l.copyWitness
}

// metadataAccessor imports or retrieves the metadata accessor for the given symbol.
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

// splitResult reports whether a multi-register result must be returned via sret pointer under MS x64 ABI.
func (l *lowerer) splitResult(t sil.Type) (int64, bool) {
	if !l.ms {
		return 0, false
	}
	if _, wide := indirect(t); wide {
		return 0, false
	}
	n, ok := directWords(t)
	if !ok || n < 2 {
		return 0, false
	}
	return int64(n) * 8, true
}

// classDestroyer synthesizes a destroyer function releasing stored property references and running deinits.
func (l *lowerer) classDestroyer(t *sil.VTable) *ir.Func {
	if t.Layout == nil {
		return nil
	}
	cl, ok := t.Layout.Underlying().(*types.Class)
	if !ok {
		return nil
	}
	var owned []ownedWord
	// A weak or unowned property's word holds a weak reference, let go
	// of as one.
	var weak []int64
	header := int64(stdlib.HeaderBytes)
	for _, f := range types.ClassFields(cl) {
		if f == nil || f.Type == nil {
			return nil
		}
		off, ok := types.Offsetof(t.Layout, f.Name, types.DefaultTarget64)
		if !ok {
			return nil
		}
		if f.Ref != "" {
			weak = append(weak, header+off)
			continue
		}
		words, ok := ownedWords(&types.Struct{Fields: []*types.Field{f}}, header+off)
		if !ok {
			return nil
		}
		owned = append(owned, words...)
	}
	// The deinits to run, the class's own first and then each
	// superclass's, which is Swift's order.
	var deinits []ir.Callee
	for c := cl; c != nil; {
		if vt := l.vtableFor(c.Name); vt != nil && vt.Deinit != "" {
			if callee, ok := l.callee[l.sym(vt.Deinit)]; ok {
				deinits = append(deinits, callee)
			} else if def, ok := l.defs[vt.Deinit]; ok {
				deinits = append(deinits, def)
			}
		}
		if c.Superclass == nil {
			break
		}
		next, isClass := c.Superclass.Underlying().(*types.Class)
		if !isClass {
			break
		}
		c = next
	}
	if len(owned) == 0 && len(deinits) == 0 && len(weak) == 0 {
		return nil
	}
	f := l.out.Func(l.sym("$sVSCdestroy_" + identSafe(t.Class)))
	f.Internal()
	obj := f.ParamPtr("object")
	b := f.Entry()
	for _, deinit := range deinits {
		b.Call(deinit, obj)
	}
	release := l.runtimeFunc(stdlib.Release, ir.NewSig().Param(ir.TypePtr))
	releaseString := l.runtimeFunc(stdlib.StringRelease, ir.NewSig().Param(ir.TypePtr))
	b = countOwned(f, b, obj, owned, releaseString, release, l.existentialCounter(false), "d")
	if len(weak) > 0 {
		weakRelease := l.runtimeFunc(stdlib.WeakRelease, ir.NewSig().Param(ir.TypePtr))
		for _, at := range weak {
			b.Call(weakRelease, b.Ptr.Load(b.Ptr.Add(obj, b.I64.Const(at))))
		}
	}
	b.Call(l.runtimeFunc(stdlib.Dealloc, ir.NewSig().Param(ir.TypePtr)), obj)
	b.Return()
	return f
}

// vtableFor is the module's table for a class, by name.
func (l *lowerer) vtableFor(class string) *sil.VTable {
	for _, t := range l.module.VTables() {
		if t.Class == class {
			return t
		}
	}
	return nil
}
