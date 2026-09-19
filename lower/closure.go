package lower

import (
	"fmt"

	"github.com/vertex-language/ir"
	"github.com/vertex-language/vsc/internal/sil"
	"github.com/vertex-language/vsc/stdlib"
	"github.com/vertex-language/vsc/types"
)

// Closures that capture. A partial_apply makes a context -- a heap object
// holding what the closure captures, a slot of two words each -- and a
// function value whose code is a forwarder. Called with the context in the
// self register, as every function value is, the forwarder reads the
// captures back out and calls the closure's body with them after the
// arguments it was given.

// captureSlot is the width of one capture in a context.
const captureSlot = 16

// closureParts are what every context of one closure body shares.
type closureParts struct {
	forwarder *ir.Func
	meta      *ir.Global
}

func (c *fn) partialApply(in *sil.Inst) error {
	res := in.Result()
	if res == nil {
		return nil
	}
	args := in.Args()
	name, ok := c.refNames[args[0]]
	if !ok {
		return c.fail(ErrUnsupported, in.Op(), "a partial application of something other than a function")
	}
	body := c.l.module.Lookup(name)
	if body == nil {
		return c.fail(ErrUnsupported, in.Op(), "no function named "+name)
	}
	sig, ok := res.Type().Formal().Underlying().(*types.Signature)
	if !ok {
		return c.fail(ErrType, in.Op(), "a partial application that is not a function value")
	}
	caps := args[1:]
	offsets, size := captureLayout(caps)
	parts, err := c.l.closureParts(name, body, sig, caps)
	if err != nil {
		return c.fail(ErrUnsupported, in.Op(), err.Error())
	}

	var obj ir.Ptr
	if at, onStack := c.contexts[res]; onStack {
		// A context that cannot escape is in storage of this body's own
		// (see stackContext), counted as immortal so that nothing the
		// closure does with it can free it.
		obj = at
		c.b.I64.Store(c.b.I64.Const(immortalCount), c.b.Ptr.Add(obj, c.b.I64.Const(8)))
		rel, err := c.l.contextReleaser(name, caps)
		if err != nil {
			return c.fail(ErrUnsupported, in.Op(), err.Error())
		}
		if rel != nil {
			if c.releasers == nil {
				c.releasers = map[*sil.Value]*ir.Func{}
			}
			c.releasers[res] = rel
		}
	} else {
		if c.l.alloc == nil {
			c.l.alloc = c.l.out.ImportFunc(c.l.sym(stdlib.Alloc),
				ir.NewSig().Param(ir.TypeI64).Ret(ir.TypePtr)).NoUnwind()
		}
		got := c.b.Call(c.l.alloc, c.b.I64.Const(size))
		if got.Len() == 0 {
			return c.fail(ErrIR, in.Op(), "the allocator returned nothing")
		}
		p, ok := got.Value(0).(ir.Ptr)
		if !ok {
			return c.fail(ErrType, in.Op(), "the allocator did not return a pointer")
		}
		obj = p
	}
	c.b.Ptr.Store(c.b.Ptr.GetAddr(parts.meta), obj)
	// The copies the context owns, which its destroyer lets go of.
	for i, a := range caps {
		// A value too wide for registers is in memory: its bytes are
		// moved into the context whole. One that arrived as its scalars --
		// a struct just returned by a call, say -- is written to the
		// storage set aside for it first, which is what wideAddress does.
		if size, wide := indirect(a.Type()); wide {
			from, inMemory, err := c.wideAddress(in, a)
			if err != nil {
				return err
			}
			if !inMemory {
				return c.fail(ErrUnsupported, in.Op(), "a wide capture that is not in memory")
			}
			for off := int64(0); off < size; off += 8 {
				at := c.b.Ptr.Add(obj, c.b.I64.Const(int64(stdlib.HeaderBytes)+offsets[i]+off))
				c.b.I64.Store(c.b.I64.Load(c.fieldAddr(from, off)), at)
			}
			continue
		}
		words, err := c.captureWords(in, a)
		if err != nil {
			return err
		}
		for k, w := range words {
			at := int64(stdlib.HeaderBytes) + offsets[i] + int64(8*k)
			if err := c.storeRegister(in, w, c.b.Ptr.Add(obj, c.b.I64.Const(at))); err != nil {
				return err
			}
		}
	}
	// An async function value names the record beside the code rather
	// than the code, because whoever calls one has to size the frame
	// before it can call it at all. The record's size is the body's:
	// the forwarder passes the context it is handed straight on. See
	// asyncsplit.go.
	var code ir.Ptr
	if sig.Async {
		code = c.b.Ptr.GetAddr(c.l.forwarderRecord(name, parts.forwarder))
	} else {
		code = c.b.Ptr.GetAddr(parts.forwarder)
	}
	c.multi[res] = []ir.Value{code, obj}
	return nil
}

// immortalCount is the refcount of an object that is never counted and
// never freed: the runtime's immortal bit (abi.h), and one reference.
const immortalCount = -1<<63 | 1

// stackContext reports whether a closure's context can live in the body's
// own storage rather than on the heap: swiftc's partial_apply [on_stack].
//
// It can when the closure cannot escape: every use of the closure value is
// a call of it, or a retain or release -- it is not stored, returned,
// passed on or captured -- which is what a body handed to withUnsafeBytes
// and its kind comes to once they are inlined. Nothing frees the context
// then; a release of one that holds references lets go of those instead
// (contextReleaser), and so it must be the only release on its path: a
// context that holds references is never retained. In an async
// body each call also has to come before the next suspension, in the
// partial_apply's own block: the value's code half is not carried across
// one, only the context in the frame is.
func stackContext(f *sil.Func, in *sil.Inst) bool {
	res := in.Result()
	if res == nil {
		return false
	}
	owns := false
	for _, a := range in.Args()[1:] {
		if !a.Type().Trivial() {
			owns = true
		}
	}
	if sig, ok := res.Type().Formal().Underlying().(*types.Signature); !ok || sig.Async {
		return false
	}
	async := f.Type() != nil && f.Type().Async
	for _, u := range res.Uses() {
		switch u.Op() {
		case sil.StrongRelease:
			continue
		case sil.StrongRetain:
			// Counted as immortal, a retained context is never let go
			// of: fine when it holds nothing, but what it holds has to be
			// released once, at the one release -- which a retain would
			// make one of several.
			if owns {
				return false
			}
			continue
		case sil.Apply:
			if u.Args()[0] != res {
				return false
			}
			for _, a := range u.Args()[1:] {
				if a == res {
					return false
				}
			}
			if async && !callBeforeSuspending(in, u) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// callBeforeSuspending reports whether call is in the same block as def,
// after it, with no call to an async function between them.
func callBeforeSuspending(def, call *sil.Inst) bool {
	if def.Block() != call.Block() {
		return false
	}
	seen := false
	for _, x := range def.Block().Insts() {
		if x == def {
			seen = true
			continue
		}
		if !seen {
			continue
		}
		if x == call {
			return true
		}
		if suspends(x) {
			return false
		}
	}
	return false
}

// suspends reports whether in may give up the thread: a call of an async
// function.
func suspends(in *sil.Inst) bool {
	if in.Op() != sil.Apply && in.Op() != sil.TryApply {
		return false
	}
	switch t := in.Args()[0].Type().Formal().Underlying().(type) {
	case *sil.FuncType:
		return t.Async
	case *types.Signature:
		return t.Async
	}
	return true
}

// forwarderRecord is the record beside a capturing async closure's
// forwarder. It does not leave the object file: the forwarder does not
// either.
func (l *lowerer) forwarderRecord(name string, fwd *ir.Func) *ir.Global {
	size, ok := l.asyncSizes[name]
	if !ok {
		size = int64(stdlib.AsyncContextBytes)
	}
	return l.asyncRecordFor(fwd, fwd.Name()+"Tu", size, false)
}

// captureLayout is where each capture sits in a context, past its header,
// and how many bytes they take together: two words at least, and a wider
// value's own bytes, rounded up to words.
func captureLayout(caps []*sil.Value) ([]int64, int64) {
	offsets := make([]int64, len(caps))
	at := int64(0)
	for i, a := range caps {
		offsets[i] = at
		width := (types.Sizeof(a.Type().Formal(), types.DefaultTarget64) + 7) &^ 7
		if width < captureSlot {
			width = captureSlot
		}
		at += width
	}
	return offsets, at
}

// captureWords is a captured value as the registers it travels in.
func (c *fn) captureWords(in *sil.Inst, v *sil.Value) ([]ir.Value, error) {
	if empty(v.Type()) {
		return nil, nil
	}
	regs, wide, err := c.gatherArg(in, v)
	if err != nil {
		return nil, err
	}
	if wide {
		return regs, nil
	}
	got, err := c.operand(in, v)
	if err != nil {
		return nil, err
	}
	return []ir.Value{got}, nil
}

// storeRegister writes one register's value where p points.
func (c *fn) storeRegister(in *sil.Inst, v ir.Value, p ir.Ptr) error {
	switch r := v.(type) {
	case ir.I1:
		// A Bool's bit, kept as a whole byte of the slot.
		c.b.I32.Store(c.b.I32.Select(r, c.b.I32.Const(1), c.b.I32.Const(0)), p)
	case ir.I64:
		c.b.I64.Store(r, p)
	case ir.I32:
		c.b.I32.Store(r, p)
	case ir.F64:
		c.b.F64.Store(r, p)
	case ir.F32:
		c.b.F32.Store(r, p)
	case ir.Ptr:
		c.b.Ptr.Store(r, p)
	default:
		return c.fail(ErrUnsupported, in.Op(), fmt.Sprintf("a capture held in a %T register", v))
	}
	return nil
}

// captureRegisters is how many registers a captured value of t is passed
// in, which is how the body's signature counts it.
func captureRegisters(t sil.Type) int {
	if empty(t) {
		return 0
	}
	// One too wide for registers reaches the body as its address.
	if _, wide := indirect(t); wide {
		return 1
	}
	if rs, ok := tupleParts(t); ok {
		return len(rs)
	}
	if n, ok := directWords(t); ok {
		return n
	}
	return 1
}

// closureParts returns the forwarder and context metadata for a body,
// making them the first time.
func (l *lowerer) closureParts(name string, body *sil.Func, sig *types.Signature, caps []*sil.Value) (closureParts, error) {
	if p, ok := l.closures[name]; ok {
		return p, nil
	}
	destroy, err := l.contextDestroyer(name, caps)
	if err != nil {
		return closureParts{}, err
	}
	// A context's metadata is a HeapMetadata: its destroyer, and no type.
	meta := l.out.Global(l.sym("$sVSCcontext_"+identSafe(name)), ir.RO, ir.Array(2, ir.StorePtr.FType())).
		Init(ir.List(ir.RelocInit(destroy), ir.Lit(ir.Int(0)))).Align(8)
	forwarder, err := l.forwarder(name, body, sig, caps)
	if err != nil {
		return closureParts{}, err
	}
	p := closureParts{forwarder: forwarder, meta: meta}
	if l.closures == nil {
		l.closures = map[string]closureParts{}
	}
	l.closures[name] = p
	return p, nil
}

// contextDestroyer releases what a context holds and frees it.
func (l *lowerer) contextDestroyer(name string, caps []*sil.Value) (*ir.Func, error) {
	return l.contextRelease(name, caps, true)
}

// contextReleaser releases what a context holds and leaves it where it is:
// the last release of a context in a body's own storage. It is nil for a
// context that holds nothing counted.
func (l *lowerer) contextReleaser(name string, caps []*sil.Value) (*ir.Func, error) {
	if l.releasers == nil {
		l.releasers = map[string]*ir.Func{}
	}
	if f, ok := l.releasers[name]; ok {
		return f, nil
	}
	owns := false
	for _, a := range caps {
		if !a.Type().Trivial() {
			owns = true
		}
	}
	var f *ir.Func
	if owns {
		var err error
		f, err = l.contextRelease(name, caps, false)
		if err != nil {
			return nil, err
		}
	}
	l.releasers[name] = f
	return f, nil
}

func (l *lowerer) contextRelease(name string, caps []*sil.Value, free bool) (*ir.Func, error) {
	var owned []ownedWord
	offsets, _ := captureLayout(caps)
	for i, a := range caps {
		one := &types.Struct{Fields: []*types.Field{{Name: "capture", Type: a.Type().Formal()}}}
		words, ok := ownedWords(one, int64(stdlib.HeaderBytes)+offsets[i])
		if !ok {
			return nil, fmt.Errorf("a capture of %s, whose copy is not a retain", a.Type())
		}
		owned = append(owned, words...)
	}
	prefix := "$sVSCdestroycontext_"
	if !free {
		prefix = "$sVSCreleasecontext_"
	}
	f := l.out.Func(l.sym(prefix + identSafe(name)))
	f.Internal()
	obj := f.ParamPtr("context")
	b := f.Entry()
	release := l.runtimeFunc(stdlib.Release, ir.NewSig().Param(ir.TypePtr))
	releaseString := l.runtimeFunc(stdlib.StringRelease, ir.NewSig().Param(ir.TypePtr))
	b = countOwned(f, b, obj, owned, releaseString, release, "d")
	if free {
		b.Call(l.runtimeFunc(stdlib.Dealloc, ir.NewSig().Param(ir.TypePtr)), obj)
	}
	b.Return()
	return f, nil
}

// forwarder is the code of a capturing closure's function value: what a
// call through it passes, then the captures read out of its context, into
// the body; and the body's results back.
func (l *lowerer) forwarder(name string, body *sil.Func, sig *types.Signature, caps []*sil.Value) (*ir.Func, error) {
	incoming, err := l.funcSig(sig)
	if err != nil {
		return nil, err
	}
	bodySig, err := l.signature(name, body.Type())
	if err != nil {
		return nil, err
	}
	callee, ok := l.callee[name]
	if !ok {
		return nil, fmt.Errorf("no function named %s", name)
	}
	f := l.out.Func(l.sym("$sVSCforward_" + identSafe(name)))
	f.Internal()
	var params []ir.Value
	for i, p := range incoming.Params() {
		v, err := forwarderParam(f, "a"+itoa(i), p.Type, p.Attrs)
		if err != nil {
			return nil, err
		}
		params = append(params, v)
	}
	for _, r := range incoming.Rets() {
		if err := forwarderRet(f, r.Type, r.Attrs); err != nil {
			return nil, err
		}
	}
	if len(params) == 0 {
		return nil, fmt.Errorf("a function value with no context parameter")
	}
	ctx, ok := params[len(params)-1].(ir.Ptr)
	if !ok {
		return nil, fmt.Errorf("a context that is not a pointer")
	}
	args := append([]ir.Value(nil), params[:len(params)-1]...)
	regs := 0
	for _, a := range caps {
		regs += captureRegisters(a.Type())
	}
	want := bodySig.Params()
	if len(want) != len(args)+regs {
		return nil, fmt.Errorf("a capturing closure whose body takes %d registers where its call passes %d and its captures %d",
			len(want), len(args), regs)
	}
	b := f.Entry()
	pos := len(args)
	offsets, _ := captureLayout(caps)
	for i, a := range caps {
		// The body reads a wide capture where the context holds it.
		if _, wide := indirect(a.Type()); wide {
			args = append(args, b.Ptr.Add(ctx, b.I64.Const(int64(stdlib.HeaderBytes)+offsets[i])))
			pos++
			continue
		}
		for k := 0; k < captureRegisters(a.Type()); k++ {
			at := b.Ptr.Add(ctx, b.I64.Const(int64(stdlib.HeaderBytes)+offsets[i]+int64(8*k)))
			var v ir.Value
			switch want[pos].Type {
			case ir.TypeI1:
				v = b.I32.Ne(b.I32.Load(at), b.I32.Const(0))
			case ir.TypeI64:
				v = b.I64.Load(at)
			case ir.TypeI32:
				v = b.I32.Load(at)
			case ir.TypeF64:
				v = b.F64.Load(at)
			case ir.TypeF32:
				v = b.F32.Load(at)
			case ir.TypePtr:
				v = b.Ptr.Load(at)
			default:
				return nil, fmt.Errorf("a capture passed in a %s register", want[pos].Type)
			}
			args = append(args, v)
			pos++
		}
	}
	// An async body does not return: its answer goes to the continuation
	// its context names, so the forwarder branches into it rather than
	// calling it and returning what it got. The context it is handed is
	// the one it passes on -- the forwarder is not a frame in the chain,
	// it is the step that reads the captures out of the closure's.
	if sig.Async {
		b.TailCall(callee, args...)
		return f, nil
	}
	got := b.Call(callee, args...)
	vals := make([]ir.Value, got.Len())
	for i := range vals {
		vals[i] = got.Value(i)
	}
	b.Return(vals...)
	return f, nil
}

func forwarderParam(f *ir.Func, name string, t ir.RegType, attrs []ir.ParamAttr) (ir.Value, error) {
	switch t {
	case ir.TypeI1:
		return f.ParamI1(name, attrs...), nil
	case ir.TypeI32:
		return f.ParamI32(name, attrs...), nil
	case ir.TypeI64:
		return f.ParamI64(name, attrs...), nil
	case ir.TypeF32:
		return f.ParamF32(name, attrs...), nil
	case ir.TypeF64:
		return f.ParamF64(name, attrs...), nil
	case ir.TypePtr:
		return f.ParamPtr(name, attrs...), nil
	}
	return nil, fmt.Errorf("a parameter in a %s register", t)
}

func forwarderRet(f *ir.Func, t ir.RegType, attrs []ir.ParamAttr) error {
	switch t {
	case ir.TypeI1:
		f.ReturnsI1(attrs...)
	case ir.TypeI32:
		f.ReturnsI32(attrs...)
	case ir.TypeI64:
		f.ReturnsI64(attrs...)
	case ir.TypeF32:
		f.ReturnsF32(attrs...)
	case ir.TypeF64:
		f.ReturnsF64(attrs...)
	case ir.TypePtr:
		f.ReturnsPtr(attrs...)
	default:
		return fmt.Errorf("a result in a %s register", t)
	}
	return nil
}
