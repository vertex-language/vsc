package lower

import (
	"github.com/vertex-language/ir"
	"github.com/vertex-language/vsc/internal/sil"
	"github.com/vertex-language/vsc/stdlib"
)

// fn tracks translation state for a single function from SIL to VIR.
type fn struct {
	l   *lowerer
	src *sil.Func
	out *ir.Func

	blocks map[*sil.Block]*ir.Block
	values map[*sil.Value]ir.Value
	// multi maps values with multi-register representations (e.g. overflow tuples, small structs).
	multi map[*sil.Value][]ir.Value
	// refs maps function_ref values to VIR Callees.
	refs map[*sil.Value]ir.Callee
	// refNames maps function_ref values to their symbol names.
	refNames map[*sil.Value]string
	// mem maps indirect values to their memory addresses.
	mem map[*sil.Value]ir.Ptr
	// wide maps memory-backed values to their entry-block stack allocations.
	wide map[*sil.Value]ir.Ptr
	// spill is storage for any other value too wide for registers, which
	// is written there when something wants its address.
	spill map[*sil.Value]ir.Ptr
	// contexts is where a closure context that does not escape lives:
	// the machine stack, or the async frame of a split body. See
	// stackContext.
	contexts map[*sil.Value]ir.Ptr
	// releasers is, for such a context that holds references, what its
	// release calls instead: see contextReleaser.
	releasers map[*sil.Value]*ir.Func
	// sret is the caller-allocated storage pointer for indirect returns.
	sret    ir.Ptr
	hasSRet bool
	// async is whether this function uses Swift's async convention, and
	// ctx is the context it was handed. See asyncsplit.go.
	async bool
	ctx   ir.Ptr
	// plan is the split this body is part of, for an async function that
	// suspends. Nil everywhere else. See asyncsplit.go.
	plan *asyncPlan
	// stubs are the resume stubs of this body's suspensions, by index:
	// what a call writes into the callee's context as where to come back
	// to.
	stubs []*ir.Func
	// witnessExtra maps witness method calls to (metadata, witnessTable) values.
	witnessExtra map[*sil.Value][2]ir.Value

	b    *ir.Block // the block being filled
	trap *ir.Block // where a failed cond_fail goes, made on demand
	// traps are the blocks a failed cond_fail with a message goes to, one
	// per message: each says why, then traps.
	traps map[string]*ir.Block
	conts int // how many times a block has been split around a trap
}

func (l *lowerer) define(f *sil.Func) error {
	// An async function that gives up its thread is not one function.
	// See asyncsplit.go.
	if p := l.plans[f.Name()]; p.hasSuspensions() {
		return l.defineSplit(f, p)
	}
	c := &fn{
		l:      l,
		src:    f,
		out:    l.defs[f.Name()],
		blocks: make(map[*sil.Block]*ir.Block),
		values: make(map[*sil.Value]ir.Value),
		multi:  make(map[*sil.Value][]ir.Value),
		refs:   make(map[*sil.Value]ir.Callee),

		refNames: make(map[*sil.Value]string),
		mem:      make(map[*sil.Value]ir.Ptr),
		wide:     make(map[*sil.Value]ir.Ptr),
		spill:    make(map[*sil.Value]ir.Ptr),

		witnessExtra: make(map[*sil.Value][2]ir.Value),

		async: f.Type() != nil && f.Type().Async,
	}

	// Create all blocks and parameters before emitting instructions.
	for _, b := range f.Blocks() {
		if err := c.openBlock(b); err != nil {
			return err
		}
	}
	if needsTrap(f) {
		c.trapBlock()
	}
	if err := c.allocSlots(); err != nil {
		return err
	}
	for _, b := range emissionOrder(f) {
		c.b = c.blocks[b]
		for _, in := range b.Insts() {
			if err := c.inst(in); err != nil {
				return err
			}
		}
	}
	// Check for any deferred IR generation errors.
	if err := c.l.out.Err(); err != nil {
		return &Error{Err: ErrIR, Func: f.SourceName(), What: err.Error()}
	}
	return nil
}

// storageKind says what a piece of a body's memory is for.
type storageKind int

const (
	// storageWide is a value held by address because it is too wide for
	// registers, or because it arrives at a block as its scalars and may
	// be wanted by address afterwards.
	storageWide storageKind = iota
	// storageStack is an alloc_stack: a variable the source declared.
	storageStack
	// storageSpill is any other value too wide for registers.
	storageSpill
	// storageContext is the context of a closure that cannot escape.
	storageContext
)

// storageWalk visits every piece of memory a body wants, in a fixed
// order, and is the one place that decides what those pieces are.
//
// There are two places to put them. An ordinary function puts them on
// its machine stack, which is what allocSlots does. A split async body
// cannot: its machine stack is gone at every suspension and something
// else is standing on it at every resume, so the same pieces go in the
// async frame instead, which the plan reserves room for. Both drive
// this walk, so that the two cannot come to disagree about what there
// is. See lower/async.go.
func (c *fn) storageWalk(yield func(k storageKind, v *sil.Value, size, align int64) error) error {
	seen := map[*sil.Value]bool{}
	wide := func(a *sil.Value) error {
		size, yes := indirect(a.Type())
		if !yes || seen[a] {
			return nil
		}
		seen[a] = true
		return yield(storageWide, a, size, storageAlign(a.Type()))
	}

	// A wide value that arrives at a block as its scalars may be wanted by
	// address -- passed to a call, returned through the caller's storage --
	// and is written back into storage of its own for that.
	for _, b := range c.src.Blocks() {
		if b.IsEntry() {
			continue
		}
		for _, a := range b.Args() {
			if err := wide(a); err != nil {
				return err
			}
		}
	}
	for _, b := range c.src.Blocks() {
		for _, in := range b.Insts() {
			if in.Op() == sil.PartialApply && stackContext(c.src, in) {
				_, size := captureLayout(in.Args()[1:])
				if err := yield(storageContext, in.Result(), int64(stdlib.HeaderBytes)+size, 16); err != nil {
					return err
				}
				continue
			}
			if in.Op() == sil.AllocStack {
				size, align, err := c.stackBytes(in)
				if err != nil {
					return err
				}
				if err := yield(storageStack, in.Result(), size, align); err != nil {
					return err
				}
				continue
			}
			// A call that may fail and whose value is too wide for
			// registers hands the value to its normal edge through
			// storage set aside here, keyed by that edge's argument.
			if in.Op() == sil.TryApply {
				for _, k := range in.Aux().Cases {
					if k.Member != "normal" || len(k.Dest.Args()) != 1 {
						continue
					}
					if err := wide(k.Dest.Args()[0]); err != nil {
						return err
					}
				}
				continue
			}
			// Reserve a slot for a wide or indirect result.
			res := in.Result()
			if res == nil || seen[res] {
				continue
			}
			switch in.Op() {
			case sil.Struct, sil.Tuple, sil.Apply, sil.TryApply:
			case sil.Enum:
				// A payload enum too wide for registers is built where
				// it is kept, a word at a time.
				if _, ok := payloadEnumOf(res.Type()); !ok {
					continue
				}
			default:
				continue
			}
			size, yes := c.outResult(in, res)
			if !yes {
				continue
			}
			if in.Op() == sil.Enum {
				size = (size + 7) / 8 * 8
			}
			seen[res] = true
			if err := yield(storageWide, res, size, storageAlign(res.Type())); err != nil {
				return err
			}
		}
	}
	// Any other value too wide for registers -- loaded, moved, borrowed --
	// may be wanted by address after arriving as its scalars.
	for _, b := range c.src.Blocks() {
		for _, in := range b.Insts() {
			for _, res := range in.Results() {
				if res == nil || seen[res] {
					continue
				}
				size, yes := indirect(res.Type())
				if !yes {
					continue
				}
				seen[res] = true
				if err := yield(storageSpill, res, size, storageAlign(res.Type())); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// allocSlots makes every piece of storage the body wants, in the entry
// block, which is where VIR admits a frame allocation and nowhere else.
//
// A split async body makes them in the async frame instead, because the
// machine stack does not survive a suspension -- see asyncStorage.
func (c *fn) allocSlots() error {
	entry := c.out.Entry()
	saved := c.b
	c.b = entry
	defer func() { c.b = saved }()

	return c.storageWalk(func(k storageKind, v *sil.Value, size, align int64) error {
		// A split async body's storage is in the async frame, at an
		// offset the plan fixed before anything was lowered. Its
		// machine stack is gone at every suspension; the frame is not.
		if c.plan != nil {
			off, there := c.plan.storage[v]
			if !there && size <= 0 {
				// Storage of no bytes -- an empty struct's -- takes no
				// room, and any address will do for it.
				c.place(k, v, c.ctx)
				return nil
			}
			if !there {
				return c.fail(ErrIR, sil.Op(""), "a piece of storage the plan left no room for")
			}
			c.place(k, v, c.b.Ptr.Add(c.ctx, c.b.I64.Const(off)))
			return nil
		}
		c.place(k, v, c.b.Ptr.Alloc(uint64(size), uint64(align)))
		return nil
	})
}

// place records where a piece of storage went.
func (c *fn) place(k storageKind, v *sil.Value, at ir.Ptr) {
	if v == nil {
		return
	}
	switch k {
	case storageStack:
		c.def(v, at)
	case storageSpill:
		c.spill[v] = at
	case storageContext:
		if c.contexts == nil {
			c.contexts = map[*sil.Value]ir.Ptr{}
		}
		c.contexts[v] = at
	default:
		c.wide[v] = at
	}
}

// spreadInto records a struct that arrived in words as the fields the
// body will read.
func (c *fn) spreadInto(v *sil.Value, regs []ir.Value) error {
	st, ok := structOf(v.Type())
	if !ok {
		return c.fail(ErrType, sil.Op(""), v.Type().String())
	}
	ls, ok := structLeaves(st)
	if !ok {
		return c.fail(ErrUnsupported, sil.Op(""), whyNoRegister(v.Type()))
	}
	words, ok := structWords(st)
	if !ok {
		return c.fail(ErrUnsupported, sil.Op(""), whyNoRegister(v.Type()))
	}
	fields, ok := c.unpackStruct(regs, words, ls)
	if !ok {
		return c.fail(ErrUnsupported, sil.Op(""), whyNoRegister(v.Type()))
	}
	if len(fields) == 1 {
		c.def(v, fields[0])
		return nil
	}
	c.multi[v] = fields
	return nil
}

// gather is the other direction: the words a struct is passed in,
// built from the registers its fields are held in.
func (c *fn) gather(in *sil.Inst, v *sil.Value) ([]ir.Value, bool, error) {
	return c.gatherWords(in, v)
}

// parts is the registers a value of more than one is held in, and is
// where c.multi is read.
//
// In a split async body a value that crosses a suspension lives in the
// frame, and is read back from it wherever it is wanted rather than
// where it was defined: the block that defined it is not on the path
// here, because control left the function in between and came back at
// the dispatch. See asyncsplit.go.
func (c *fn) parts(v *sil.Value) ([]ir.Value, bool) {
	if got, ok := c.loadSlotParts(v); ok {
		return got, true
	}
	got, ok := c.multi[v]
	return got, ok
}

// multiPart reports whether a value is held in more than one register:
// a struct as its fields, a tuple as its elements.
//
// It is not the same question as how many registers the value travels
// in. A struct of two int32s is one word across a call and two registers
// in a body, and reading it back out of the frame as the one word would
// hand a field's worth of code a value twice its width.
func multiPart(t sil.Type) bool {
	if _, ok := tupleParts(t); ok {
		return true
	}
	ls, ok := leavesOf(t)
	return ok && len(ls) > 1
}

// loadSlotParts reads a slotted value of more than one register out of
// the frame, in the form the body holds it: a tuple's elements, or a
// struct's fields unpacked from the words it travels in.
func (c *fn) loadSlotParts(v *sil.Value) ([]ir.Value, bool) {
	if c.plan == nil || v == nil || !multiPart(v.Type()) {
		return nil, false
	}
	off, slotted := c.plan.slot[v]
	if !slotted {
		return nil, false
	}
	rs, ok := frameRegs(v.Type())
	if !ok || len(rs) == 0 {
		return nil, false
	}
	words := make([]ir.Value, 0, len(rs))
	for i, r := range rs {
		words = append(words, c.loadReg(r, c.b.Ptr.Add(c.ctx, c.b.I64.Const(off+int64(8*i)))))
	}
	// A tuple is its elements, one register each, and is held that way.
	if _, isTuple := tupleParts(v.Type()); isTuple {
		return words, true
	}
	ls, ok := leavesOf(v.Type())
	if !ok {
		return nil, false
	}
	st, _ := structOf(v.Type())
	ws, ok := structWords(st)
	if !ok {
		return nil, false
	}
	fields, ok := c.unpackStruct(words, ws, ls)
	if !ok {
		return nil, false
	}
	return fields, true
}

// gatherArg returns argument registers for a call, passing tuple elements directly.
func (c *fn) gatherArg(in *sil.Inst, v *sil.Value) ([]ir.Value, bool, error) {
	if rs, ok := tupleParts(v.Type()); ok {
		parts, held := c.parts(v)
		if !held {
			got, err := c.operand(in, v)
			if err != nil {
				return nil, false, err
			}
			parts = []ir.Value{got}
		}
		if len(parts) != len(rs) {
			return nil, false, c.fail(ErrUnsupported, in.Op(),
				"a tuple whose elements are not one register each")
		}
		return parts, true, nil
	}
	return c.gatherWords(in, v)
}

// gatherWords packs a composite value into 8-byte word registers for passing.
func (c *fn) gatherWords(in *sil.Inst, v *sil.Value) ([]ir.Value, bool, error) {
	ls, ok := leavesOf(v.Type())
	if !ok || len(ls) < 2 {
		return nil, false, nil
	}
	st, _ := structOf(v.Type())
	words, ok := structWords(st)
	if !ok {
		return nil, false, c.fail(ErrUnsupported, in.Op(), whyNoRegister(v.Type()))
	}
	// A value that lives in memory is already in the form a call
	// wants: its bytes, cut into words. So the words are loaded
	// rather than packed out of registers it is not in -- an array's
	// element comes back through storage, and a String read out of
	// one is this.
	if from, ok := c.mem[v]; ok {
		regs := make([]ir.Value, 0, len(words))
		for i := range words {
			regs = append(regs, c.b.I64.Load(c.fieldAddr(from, int64(i)*8)))
		}
		return regs, true, nil
	}
	fields, ok := c.parts(v)
	if !ok {
		// One register standing for the whole struct, which happens
		// where every field but one holds nothing.
		got, err := c.operand(in, v)
		if err != nil {
			return nil, false, err
		}
		fields = []ir.Value{got}
	}
	regs, ok := c.packStruct(fields, words)
	if !ok {
		return nil, false, c.fail(ErrUnsupported, in.Op(), whyNoRegister(v.Type()))
	}
	return regs, true, nil
}

func (c *fn) openBlock(b *sil.Block) error {
	if b.IsEntry() {
		out := c.out.Entry()
		c.blocks[b] = out
		// Unpack incoming composite parameters at the top of the entry block.
		c.b = out
		params := c.out.Params()
		i := 0
		// Caller-provided indirect return storage (sret) first, then
		// the context an async function is handed: the frame it keeps
		// anything that outlives a suspension in, and the continuation
		// it returns through. That is the order the signature declares
		// them in, and swiftc's. See lowerer.signature.
		if c.returnsIndirectly() && len(params) > i {
			if p, ok := ir.Wrap(params[i]).(ir.Ptr); ok {
				c.sret, c.hasSRet = p, true
				i++
			}
		}
		if c.async && len(params) > i {
			if p, ok := ir.Wrap(params[i]).(ir.Ptr); ok {
				c.ctx = p
				i++
			}
		}
		for _, a := range b.Args() {
			if empty(a.Type()) {
				continue
			}
			// Indirect parameter passed by address.
			if _, yes := indirect(a.Type()); yes {
				if i >= len(params) {
					return c.fail(ErrUnsupported, sil.Op(""), "more entry arguments than parameters")
				}
				p, ok := ir.Wrap(params[i]).(ir.Ptr)
				if !ok {
					return c.fail(ErrType, sil.Op(""), "a wide argument that is not a pointer")
				}
				c.mem[a] = p
				i++
				continue
			}
			// A tuple arrived as its elements, one parameter each.
			if rs, ok := tupleParts(a.Type()); ok {
				if i+len(rs) > len(params) {
					return c.fail(ErrUnsupported, sil.Op(""),
						"more entry arguments than parameters")
				}
				parts := make([]ir.Value, 0, len(rs))
				for range rs {
					parts = append(parts, ir.Wrap(params[i]))
					i++
				}
				if len(parts) == 1 {
					c.def(a, parts[0])
				} else {
					c.multi[a] = parts
				}
				continue
			}
			n, wide := directWords(a.Type())
			if !wide {
				if i >= len(params) {
					return c.fail(ErrUnsupported, sil.Op(""), "more entry arguments than parameters")
				}
				c.values[a] = ir.Wrap(params[i])
				i++
				continue
			}
			if i+n > len(params) {
				return c.fail(ErrUnsupported, sil.Op(""), "more entry arguments than parameters")
			}
			regs := make([]ir.Value, 0, n)
			for _, p := range params[i : i+n] {
				regs = append(regs, ir.Wrap(p))
			}
			i += n
			if err := c.spreadInto(a, regs); err != nil {
				return err
			}
		}
		return nil
	}
	out := c.out.Block(b.Label())
	c.blocks[b] = out
	for i, a := range b.Args() {
		if empty(a.Type()) {
			continue
		}
		// One argument may be several registers: a case's payload
		// that is a tuple arrives as its scalars, the way a struct
		// parameter arrives as its words. What the block declares has
		// to match what every edge into it passes.
		if ls, ok := leavesOf(a.Type()); ok && len(ls) > 1 {
			parts := make([]ir.Value, 0, len(ls))
			for j, l := range ls {
				r, ok := machineOf(l.typ)
				if !ok {
					return c.fail(ErrType, sil.Op(""), l.typ.String())
				}
				parts = append(parts, out.Param(r.reg,
					b.Label()+"."+itoa(i)+"."+itoa(j)))
			}
			c.multi[a] = parts
			continue
		}
		r, ok := machine(a.Type())
		if !ok {
			return c.fail(ErrType, sil.Op(""), a.Type().String())
		}
		c.values[a] = out.Param(r.reg, b.Label()+"."+itoa(i))
	}
	return nil
}

// value is the register a SIL value is held in. A value of a type that
// holds nothing has no register, and ok is false: callers drop it
// rather than passing a poison value along.
func (c *fn) value(v *sil.Value) (ir.Value, bool) {
	if v == nil || empty(v.Type()) {
		return nil, false
	}
	// In a split async body a value that crosses a suspension lives in
	// the frame, and is read from it wherever it is wanted. The
	// definition it came from is not on the path here: control left the
	// function in between and came back at the dispatch.
	//
	// One held in several registers is not one of these: it is read back
	// by parts, and answering with a word here would be answering with
	// the wrong thing.
	if c.plan != nil && !multiPart(v.Type()) {
		if off, slotted := c.plan.slot[v]; slotted && valueWords(v) == 1 {
			return c.loadSlot(v, off), true
		}
	}
	got, ok := c.values[v]
	return got, ok
}

// loadSlot reads a frame slot as the register type the value travels in.
func (c *fn) loadSlot(v *sil.Value, off int64) ir.Value {
	r, ok := machine(v.Type())
	at := c.b.Ptr.Add(c.ctx, c.b.I64.Const(off))
	if !ok {
		return c.b.I64.Load(at)
	}
	return c.loadReg(r.reg, at)
}

// loadReg reads one word of the frame as the register it stands for.
func (c *fn) loadReg(r ir.RegType, at ir.Ptr) ir.Value {
	switch r {
	case ir.TypeI32:
		return c.b.I32.Load(at)
	case ir.TypeI1:
		return c.b.I32.Ne(c.b.I32.Load(at), c.b.I32.Const(0))
	case ir.TypePtr:
		return c.b.Ptr.Load(at)
	case ir.TypeF64:
		return c.b.F64.Load(at)
	case ir.TypeF32:
		return c.b.F32.Load(at)
	}
	return c.b.I64.Load(at)
}

// storeSlot writes a value to its frame slot, which is what defining a
// value that crosses a suspension does.
//
// It reads c.multi rather than parts, because what is being written
// down is what has just been computed: parts would read the slot back,
// which is the thing this is filling in.
func (c *fn) storeSlot(in *sil.Inst, v *sil.Value) error {
	if c.plan == nil || v == nil {
		return nil
	}
	off, slotted := c.plan.slot[v]
	if !slotted {
		return nil
	}
	rs, ok := frameRegs(v.Type())
	if !ok || len(rs) == 0 {
		return nil
	}
	at := func(i int) ir.Ptr {
		return c.b.Ptr.Add(c.ctx, c.b.I64.Const(off+int64(8*i)))
	}
	if !multiPart(v.Type()) {
		got, held := c.values[v]
		if !held {
			return nil
		}
		return storeWord(c.b, got, at(0))
	}
	fields, held := c.multi[v]
	if !held {
		return nil
	}
	words := fields
	if _, isTuple := tupleParts(v.Type()); !isTuple {
		st, _ := structOf(v.Type())
		ws, ok := structWords(st)
		if !ok {
			return c.fail(ErrUnsupported, opOf(in), whyNoRegister(v.Type()))
		}
		packed, ok := c.packStruct(fields, ws)
		if !ok {
			return c.fail(ErrUnsupported, opOf(in), whyNoRegister(v.Type()))
		}
		words = packed
	}
	if len(words) != len(rs) {
		return c.fail(ErrUnsupported, opOf(in),
			"a value this pass cannot carry across a suspension")
	}
	for i, w := range words {
		if err := storeWord(c.b, w, at(i)); err != nil {
			return err
		}
	}
	return nil
}

// opOf is an instruction's opcode, for a diagnostic about a value that
// may have arrived as a block argument rather than from an instruction.
func opOf(in *sil.Inst) sil.Op {
	if in == nil {
		return sil.Op("")
	}
	return in.Op()
}

// def records what a SIL value became.
func (c *fn) def(v *sil.Value, got ir.Value) {
	if v == nil {
		return
	}
	c.values[v] = got
}

// forward aliases dst to src's register representation.
func (c *fn) forward(dst, src *sil.Value) {
	if got, ok := c.value(src); ok {
		c.def(dst, got)
		return
	}
	// Forward multi-register values.
	if parts, ok := c.parts(src); ok {
		c.multi[dst] = parts
		return
	}
	if p, ok := c.mem[src]; ok {
		c.mem[dst] = p
	}
}

// args are the registers a branch passes, in order, minus the ones
// that hold nothing.
func (c *fn) args(vs []*sil.Value) []ir.Value {
	out := make([]ir.Value, 0, len(vs))
	for _, v := range vs {
		// One value may be several registers -- an optional is its
		// payload and a tag -- and the block it goes to declares one
		// parameter for each. What the branch supplies has to match.
		if parts, ok := c.parts(v); ok {
			out = append(out, parts...)
			continue
		}
		if got, ok := c.value(v); ok {
			out = append(out, got)
			continue
		}
		// A value that lives in memory -- a struct a call answered by
		// address -- is read out field by field, as a load reads it,
		// since the block takes it as registers.
		if from, ok := c.mem[v]; ok {
			if ls, ok := leavesOf(v.Type()); ok {
				loaded := true
				fields := make([]ir.Value, 0, len(ls))
				for _, l := range ls {
					fr, ok := machineOf(l.typ)
					if !ok {
						loaded = false
						break
					}
					got, err := c.loadScalar(nil, c.fieldAddr(from, l.offset), fr)
					if err != nil {
						loaded = false
						break
					}
					fields = append(fields, got)
				}
				if loaded {
					out = append(out, fields...)
					continue
				}
			}
		}
	}
	return out
}

// trapBlockFor is the block a failed cond_fail with this message goes to:
// it hands the message to the runtime, which flushes what was printed and
// writes `Fatal error: ...` as Swift does, and then traps.
func (c *fn) trapBlockFor(message string) *ir.Block {
	// A device has no runtime to say it with: the trap alone.
	if message == "" || c.l.device {
		return c.trapBlock()
	}
	if b, ok := c.traps[message]; ok {
		return b
	}
	if c.traps == nil {
		c.traps = map[string]*ir.Block{}
	}
	b := c.out.Block("fatal" + itoa(len(c.traps)))
	c.traps[message] = b
	text := c.l.cstring(fatalMessageSymbol(message), message)
	text.Internal()
	fatal := c.l.runtimeFunc(stdlib.Fatal, ir.NewSig().Param(ir.TypePtr))
	b.Call(fatal, b.Ptr.GetAddr(text))
	b.Trap()
	return b
}

// fatalMessageSymbol names the text of a cond_fail's message: the same
// for the same message, so a module holds each once.
func fatalMessageSymbol(message string) string {
	var h uint64 = 14695981039346656037
	for i := 0; i < len(message); i++ {
		h ^= uint64(message[i])
		h *= 1099511628211
	}
	const digits = "0123456789abcdef"
	out := []byte("$vsc_fatal_")
	for i := 60; i >= 0; i -= 4 {
		out = append(out, digits[(h>>uint(i))&0xF])
	}
	return string(out)
}

// trapBlock returns the function's shared trap block for failed assertions or overflow.
func (c *fn) trapBlock() *ir.Block {
	if c.trap == nil {
		c.trap = c.out.Block("trap")
		c.trap.Trap()
	}
	return c.trap
}

// emissionOrder is the order a body's blocks are lowered in: reverse
// postorder, so that a block is lowered after every block that dominates
// it and a value is defined before any block using it is reached, however
// the blocks were created; then any block nothing reaches.
func emissionOrder(f *sil.Func) []*sil.Block {
	var post []*sil.Block
	seen := map[*sil.Block]bool{}
	var walk func(b *sil.Block)
	walk = func(b *sil.Block) {
		if b == nil || seen[b] {
			return
		}
		seen[b] = true
		if t := b.Term(); t != nil {
			succs := t.Successors()
			for i := len(succs) - 1; i >= 0; i-- {
				walk(succs[i])
			}
		}
		post = append(post, b)
	}
	walk(f.Entry())
	order := make([]*sil.Block, 0, len(f.Blocks()))
	for i := len(post) - 1; i >= 0; i-- {
		order = append(order, post[i])
	}
	for _, b := range f.Blocks() {
		if !seen[b] {
			order = append(order, b)
		}
	}
	return order
}

func needsTrap(f *sil.Func) bool {
	// Only a cond_fail with no message goes to the shared block; one with
	// a message gets a block of its own, made when it is reached.
	for _, b := range f.Blocks() {
		for _, in := range b.Insts() {
			if in.Op() == sil.CondFail && in.Aux().Text == "" {
				return true
			}
		}
	}
	return false
}

func (c *fn) fail(err error, op sil.Op, what string) error {
	// An error already recorded in the module makes every builder answer
	// with nothing, and what fails next is whatever happened to read one
	// of those answers. Say what was actually wrong rather than blaming
	// the instruction that came after it.
	if prior := c.l.out.Err(); prior != nil {
		return &Error{Err: ErrIR, Func: c.src.SourceName(), Op: op, What: prior.Error()}
	}
	return &Error{Err: err, Func: c.src.SourceName(), Op: op, What: what}
}

// returnsIndirectly reports whether this function's result comes back
// through storage the caller set aside.
func (c *fn) returnsIndirectly() bool {
	t := c.src.Type()
	if t == nil {
		return false
	}
	for _, res := range t.Results {
		if res.Convention == sil.ResultOut {
			return true
		}
		if _, yes := indirect(res.Type); yes {
			return true
		}
		if _, split := c.l.splitResult(res.Type); split {
			return true
		}
	}
	return false
}
