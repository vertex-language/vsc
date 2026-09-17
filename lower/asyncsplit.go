package lower

import (
	"github.com/vertex-language/ir"
	"github.com/vertex-language/vsc/internal/sil"
	"github.com/vertex-language/vsc/stdlib"
)

// Cutting an async function into the functions it becomes.
//
// An async function that gives up its thread cannot be one function: it
// leaves at a suspension and comes back where the callee's context says,
// and a function cannot be re-entered in the middle. So it becomes
//
//	f          the prologue: arguments into the frame, then into the body
//	f…TA_      every block of it, behind a dispatch on where to carry on
//	f…TQn_     one per suspension: what the call answered into the frame,
//	           the call's frame freed, back to the body
//
// Swift emits a funclet per suspension instead, each holding the code
// that follows it. One body behind a dispatch is the other way of
// splitting a coroutine, and it is the one that survives a loop with an
// `await` in it: the code after a suspension branches back to the code
// before it, which is an ordinary branch while both are in one function
// and an impossible one when they are not.
//
// None of that is visible across a call. What a caller writes into
// `resumeParent` is an address, and whether it belongs to a funclet
// holding the rest of the function or to a stub that jumps into a shared
// body is this compiler's business. The protocol either way is Swift's;
// see docs/vertex_swift_async.md.
//
// Anything that has to cross a boundary lives in the frame and is read
// from it at every use. That is more loads than a smarter pass would
// emit, and it is right whatever the control flow does, which is worth
// more here than the loads cost.

func bodyName(f *sil.Func) string { return f.Name() + "TA_" }

func resumeName(f *sil.Func, i int) string { return f.Name() + "TQ" + itoa(i) + "_" }

// stateSlot is where the body keeps the number of the dispatch arm to
// take: 0 on the way in, and one past a suspension's index after it. It
// is the first word of the frame, so reading it needs nothing else
// worked out.
func stateSlot() int64 { return int64(stdlib.AsyncContextBytes) }

// defineSplit lowers an async function that suspends into the several
// functions it becomes.
func (l *lowerer) defineSplit(f *sil.Func, p *asyncPlan) error {
	body := l.out.Func(l.sym(bodyName(f)))
	body.Internal()
	body.ParamPtr("ctx", ir.SwiftAsync)

	// Every piece is made before any is filled: the body refers to the
	// stubs by address, and a second Func of the same name would be a
	// second function rather than the same one again.
	stubs := make([]*ir.Func, len(p.suspends))
	for i, s := range p.suspends {
		st := l.out.Func(l.sym(resumeName(f, i)))
		st.Internal()
		st.ParamPtr("callee", ir.SwiftAsync)
		// The answer arrives in the registers it travels in, which is
		// what the callee's own return hands the continuation. Their
		// types have to agree with it or the two ends of one call
		// disagree about what is where.
		for _, r := range s.results {
			rs, ok := frameRegs(r.Type())
			if !ok {
				return &Error{Err: ErrType, Func: f.SourceName(), What: r.Type().String()}
			}
			for _, reg := range rs {
				st.ParamOf(reg, "r"+itoa(len(st.Params())-1))
			}
		}
		// A call that may fail answers with the error last, in the self
		// register, null where it did not. See continuationType.
		if s.throws {
			st.ParamPtr("err", ir.SwiftSelf)
		}
		stubs[i] = st
	}

	if err := l.definePrologue(f, p, body); err != nil {
		return err
	}
	if err := l.defineAsyncBody(f, p, body, stubs); err != nil {
		return err
	}
	for i, s := range p.suspends {
		if err := l.defineResume(f, p, s, body, stubs[i]); err != nil {
			return err
		}
	}
	return nil
}

// definePrologue fills f itself: the arguments into the frame, where the
// body reads them, and then into the body.
//
// They have to be written down because the body takes a context and
// nothing else: they arrive once, in registers, and every arm of the
// dispatch after the first is reached without them.
func (l *lowerer) definePrologue(f *sil.Func, p *asyncPlan, body *ir.Func) error {
	out := l.defs[f.Name()]
	if out == nil {
		return &Error{Err: ErrUnsupported, Func: f.SourceName(), What: "no declaration to define"}
	}
	b := out.Entry()
	params := out.Params()
	if len(params) == 0 {
		return &Error{Err: ErrUnsupported, Func: f.SourceName(), What: "an async function with no context"}
	}
	// The caller's result storage comes first where there is one, then
	// the context, as the signature declares them. See
	// lowerer.signature.
	at := 0
	var sret ir.Ptr
	if p.hasSRet {
		got, ok := ir.Wrap(params[at]).(ir.Ptr)
		if !ok {
			return &Error{Err: ErrType, Func: f.SourceName(), What: "result storage that is not a pointer"}
		}
		sret = got
		at++
	}
	if at >= len(params) {
		return &Error{Err: ErrUnsupported, Func: f.SourceName(), What: "an async function with no context"}
	}
	ctx, ok := ir.Wrap(params[at]).(ir.Ptr)
	if !ok {
		return &Error{Err: ErrType, Func: f.SourceName(), What: "a context that is not a pointer"}
	}
	at++
	if p.hasSRet {
		// Written into the frame like an argument: the body is entered
		// with a context and nothing else.
		b.Ptr.Store(sret, b.Ptr.Add(ctx, b.I64.Const(p.sret)))
	}
	for _, a := range p.params {
		n := valueWords(a)
		off, slotted := p.slot[a]
		for k := 0; k < n; k++ {
			if at >= len(params) {
				return &Error{Err: ErrUnsupported, Func: f.SourceName(),
					What: "an argument this pass cannot find a register for"}
			}
			if slotted {
				if err := storeWord(b, ir.Wrap(params[at]), b.Ptr.Add(ctx, b.I64.Const(off+int64(8*k)))); err != nil {
					return &Error{Err: ErrUnsupported, Func: f.SourceName(), What: err.Error()}
				}
			}
			at++
		}
	}

	b.I64.Store(b.I64.Const(0), b.Ptr.Add(ctx, b.I64.Const(stateSlot())))
	b.TailCall(body, ctx)
	return nil
}

// defineResume fills one resume stub: what the call answered into the
// frame, the call's frame freed, and back to the body.
func (l *lowerer) defineResume(f *sil.Func, p *asyncPlan, s *suspension, body, out *ir.Func) error {
	b := out.Entry()
	params := out.Params()
	callee, ok := ir.Wrap(params[0]).(ir.Ptr)
	if !ok {
		return &Error{Err: ErrType, Func: f.SourceName(), What: "a context that is not a pointer"}
	}
	// This function's own context is the callee's first word: whoever
	// made the call wrote it there. See the protocol in async.go.
	mine := b.Ptr.Load(callee)

	at := 1
	for _, r := range s.results {
		off, slotted := p.slot[r]
		for k := 0; k < valueWords(r); k++ {
			if at >= len(params) {
				break
			}
			if slotted {
				if err := storeWord(b, ir.Wrap(params[at]), b.Ptr.Add(mine, b.I64.Const(off+int64(8*k)))); err != nil {
					return &Error{Err: ErrUnsupported, Func: f.SourceName(), What: err.Error()}
				}
			}
			at++
		}
	}

	// The error, where there may be one, so that the body can branch on
	// it once it is back.
	if s.throws {
		if at >= len(params) {
			return &Error{Err: ErrIR, Func: f.SourceName(), What: "a failing call with no error parameter"}
		}
		if err := storeWord(b, ir.Wrap(params[at]), b.Ptr.Add(mine, b.I64.Const(s.errSlot))); err != nil {
			return &Error{Err: ErrUnsupported, Func: f.SourceName(), What: err.Error()}
		}
	}

	// The call is over, so its frame is too. Freeing it here rather than
	// later keeps the stack discipline the allocator relies on: this is
	// the innermost frame at this moment and nothing else has been
	// allocated since.
	dealloc := l.runtimeFunc(stdlib.TaskDealloc, ir.NewSig().Param(ir.TypePtr))
	b.Call(dealloc, callee)
	b.TailCall(body, mine)
	return nil
}

// storeWord writes one register into the frame.
func storeWord(b *ir.Block, v ir.Value, at ir.Ptr) error {
	switch x := v.(type) {
	case ir.I64:
		b.I64.Store(x, at)
	case ir.I32:
		b.I32.Store(x, at)
	case ir.I1:
		b.I32.Store(b.I32.ZExtI1(x), at)
	case ir.Ptr:
		b.Ptr.Store(x, at)
	case ir.F64:
		b.F64.Store(x, at)
	case ir.F32:
		b.F32.Store(x, at)
	}
	return nil
}

// defineAsyncBody fills the body: a dispatch on where to carry on, then
// every block of the function, with each suspension turned into a call
// that does not come back here.
func (l *lowerer) defineAsyncBody(f *sil.Func, p *asyncPlan, body *ir.Func, stubs []*ir.Func) error {
	c := &fn{
		l:      l,
		src:    f,
		out:    body,
		blocks: make(map[*sil.Block]*ir.Block),
		values: make(map[*sil.Value]ir.Value),
		multi:  make(map[*sil.Value][]ir.Value),
		refs:   make(map[*sil.Value]ir.Callee),

		refNames: make(map[*sil.Value]string),
		mem:      make(map[*sil.Value]ir.Ptr),
		wide:     make(map[*sil.Value]ir.Ptr),
		spill:    make(map[*sil.Value]ir.Ptr),

		witnessExtra: make(map[*sil.Value][2]ir.Value),

		async: true,
		plan:  p,
		stubs: stubs,
	}
	params := body.Params()
	ctx, ok := ir.Wrap(params[0]).(ir.Ptr)
	if !ok {
		return &Error{Err: ErrType, Func: f.SourceName(), What: "a context that is not a pointer"}
	}
	c.ctx = ctx

	// The entry block is the dispatch. Every block of the function gets
	// one of its own, including the one SIL calls the entry, because
	// coming in at the top is only one of the ways in.
	dispatch := body.Entry()
	for i, b := range f.Blocks() {
		if b.IsEntry() {
			// The function's own entry is an ordinary block here: the
			// dispatch is what the body is entered at, and the top of
			// the function is only one of the arms.
			c.blocks[b] = body.Block("b" + itoa(i))
			continue
		}
		// Every other block keeps its arguments, which are the
		// parameters of the block it becomes. A branch to it is an
		// ordinary branch and carries them.
		if err := c.openBlock(b); err != nil {
			return err
		}
	}
	if needsTrap(f) {
		c.b = dispatch
		c.trapBlock()
	}
	c.b = dispatch
	if err := c.allocSlots(); err != nil {
		return err
	}
	// The caller's result storage, and any parameter passed by address:
	// pointers the prologue wrote down, read back once. The dispatch
	// runs on the way in and at every resume, so what it works out here
	// is right on every path.
	if p.hasSRet {
		at, ok := c.loadReg(ir.TypePtr, c.b.Ptr.Add(ctx, c.b.I64.Const(p.sret))).(ir.Ptr)
		if !ok {
			return &Error{Err: ErrType, Func: f.SourceName(), What: "result storage that is not a pointer"}
		}
		c.sret, c.hasSRet = at, true
	}
	if err := c.asyncMemParams(f, p); err != nil {
		return err
	}

	// Lower every block. A suspension ends the block it is in and opens
	// a new one for what follows, which is where the dispatch comes back
	// to.
	resumeAt := make([]*ir.Block, len(p.suspends))
	for _, b := range f.Blocks() {
		c.b = c.blocks[b]
		// An argument that crosses a suspension is written down where
		// the block is entered, because after one it is read out of the
		// frame rather than out of the parameter it arrived in.
		if !b.IsEntry() {
			for _, a := range b.Args() {
				if err := c.storeSlot(nil, a); err != nil {
					return err
				}
				if err := c.settle(nil, a); err != nil {
					return err
				}
			}
		}
		for _, in := range b.Insts() {
			if s := p.suspensionAt(in); s != nil {
				after := body.Block("resume" + itoa(s.index))
				if err := c.emitSuspend(s, after); err != nil {
					return err
				}
				resumeAt[s.index] = after
				c.b = after
				continue
			}
			if err := c.inst(in); err != nil {
				return err
			}
			for _, r := range in.Results() {
				if err := c.storeSlot(in, r); err != nil {
					return err
				}
				if err := c.settle(in, r); err != nil {
					return err
				}
			}
		}
	}

	// The dispatch, now that every arm exists: state 0 is the top of the
	// function, and one past a suspension's index is what follows it.
	// A chain of tests rather than a table, because there are as many
	// arms as the function has awaits and that is a small number.
	top := c.blocks[f.Blocks()[0]]
	at := dispatch
	for i := range p.suspends {
		if resumeAt[i] == nil {
			return &Error{Err: ErrUnsupported, Func: f.SourceName(),
				What: "a suspension with nowhere to come back to"}
		}
		state := at.I64.Load(at.Ptr.Add(ctx, at.I64.Const(stateSlot())))
		cond := at.I64.Eq(state, at.I64.Const(int64(i+1)))
		els := top
		if i+1 < len(p.suspends) {
			els = body.Block("dispatch" + itoa(i+1))
		}
		at.BrIf(cond, resumeAt[i].To(), els.To())
		at = els
	}
	if len(p.suspends) == 0 {
		dispatch.Br(top.To())
	}

	if err := c.l.out.Err(); err != nil {
		return &Error{Err: ErrIR, Func: f.SourceName(), What: err.Error()}
	}
	return nil
}

// settle moves a value that has storage in the frame out of registers and
// into that storage, the moment it is defined.
//
// Such a value -- one too wide for registers, loaded or taken apart into
// its scalars -- is held both ways in an ordinary body: as scalars for
// the code that wants scalars, and written into its storage only when
// something wants its address. In a split body the scalars do not last:
// a use after a suspension is in a different run of the body, and the
// registers it would read were filled in a run that has returned. The
// storage does last, because it is in the frame. So the value goes there
// at once and is read from there, which is what every use of one held in
// memory already knows how to do.
func (c *fn) settle(in *sil.Inst, v *sil.Value) error {
	if v == nil || c.plan == nil {
		return nil
	}
	if _, held := c.plan.storage[v]; !held {
		return nil
	}
	if _, inMemory := c.mem[v]; inMemory {
		return nil
	}
	if _, scalars := c.multi[v]; !scalars {
		return nil
	}
	if in == nil {
		// A block argument: the storage is written from its parameters
		// the same way.
		slot, ok := c.wide[v]
		if !ok {
			slot, ok = c.spill[v]
		}
		if !ok {
			return nil
		}
		if _, err := c.storeLeaves(nil, v, slot); err != nil {
			return err
		}
		c.mem[v] = slot
		delete(c.multi, v)
		return nil
	}
	if _, _, err := c.wideAddress(in, v); err != nil {
		return err
	}
	if _, ok := c.mem[v]; ok {
		delete(c.multi, v)
	}
	return nil
}

// asyncMemParams reads back the parameters that were passed by address.
//
// Such a parameter is a pointer, and the storage it points at is the
// caller's -- the caller is async too, so that storage is in its own
// frame and outlives every suspension here. What has to be kept is the
// pointer, which the prologue wrote into the frame.
func (c *fn) asyncMemParams(f *sil.Func, p *asyncPlan) error {
	blocks := f.Blocks()
	if len(blocks) == 0 {
		return nil
	}
	for _, a := range blocks[0].Args() {
		if _, yes := indirect(a.Type()); !yes {
			continue
		}
		off, slotted := p.slot[a]
		if !slotted {
			return c.fail(ErrIR, sil.Op(""), "a parameter passed by address with no slot")
		}
		at, ok := c.loadReg(ir.TypePtr, c.b.Ptr.Add(c.ctx, c.b.I64.Const(off))).(ir.Ptr)
		if !ok {
			return c.fail(ErrType, sil.Op(""), "a parameter address that is not a pointer")
		}
		c.mem[a] = at
	}
	return nil
}

// emitSuspend turns a call to an async function into what it is: the
// callee's context made and pointed back here, and a tail call that does
// not return.
func (c *fn) emitSuspend(s *suspension, after *ir.Block) error {
	in := s.at
	args := in.Args()

	// Where to carry on, so the dispatch knows on the way back.
	c.b.I64.Store(c.b.I64.Const(int64(s.index+1)),
		c.b.Ptr.Add(c.ctx, c.b.I64.Const(stateSlot())))

	// Who is being called, and how big a context it wants. Both come
	// from the record beside it where the callee is a value or is in
	// another module, and from this module's own working out where it
	// is one of its functions. See asyncRecord.
	target, through, closureCtx, want, err := c.suspensionTarget(s)
	if err != nil {
		return err
	}
	alloc := c.l.runtimeFunc(stdlib.TaskAlloc, ir.NewSig().Param(ir.TypeI64).Ret(ir.TypePtr))
	got := c.b.Call(alloc, want)
	if got.Len() == 0 {
		// An error already recorded makes every builder answer with
		// nothing, so say what it was rather than blaming the call
		// that happened to come after it.
		if err := c.l.out.Err(); err != nil {
			return c.fail(ErrIR, in.Op(), err.Error())
		}
		return c.fail(ErrIR, in.Op(), "the task allocator returned nothing")
	}
	child, ok := got.Value(0).(ir.Ptr)
	if !ok {
		return c.fail(ErrType, in.Op(), "the task allocator did not return a pointer")
	}

	// Point it back here: the parent context, and the stub to come back
	// to. The stub's address is what makes this a suspension rather than
	// a call -- the callee returns into it, not into us.
	c.b.Ptr.Store(c.ctx, child)
	if s.index >= len(c.stubs) || c.stubs[s.index] == nil {
		return c.fail(ErrIR, in.Op(), "a suspension with no resume stub")
	}
	c.b.Ptr.Store(c.b.Ptr.GetAddr(c.stubs[s.index]), c.b.Ptr.Add(child, c.b.I64.Const(8)))
	// Keep the callee's context, so the stub can free it.
	c.b.Ptr.Store(child, c.b.Ptr.Add(c.ctx, c.b.I64.Const(s.calleeSlot)))

	// The arguments, then away. A result too wide for registers comes
	// back through storage set aside for it, whose address goes first,
	// as it does for an ordinary call.
	var call []ir.Value
	if out, ok := c.answerStorage(s); ok {
		call = append(call, out)
	}
	call = append(call, child)
	for _, a := range args[1:] {
		if empty(a.Type()) {
			continue
		}
		if _, yes := indirect(a.Type()); yes {
			p, ok, err := c.wideAddress(in, a)
			if err != nil {
				return err
			}
			if !ok {
				return c.fail(ErrUnsupported, in.Op(), "a struct passed by address that is not in memory")
			}
			call = append(call, p)
			continue
		}
		regs, wide, err := c.gatherArg(in, a)
		if err != nil {
			return err
		}
		if wide {
			call = append(call, regs...)
			continue
		}
		got, err := c.operand(in, a)
		if err != nil {
			return err
		}
		call = append(call, got)
	}
	// A function value is called with its captures in the self register,
	// as a synchronous one is.
	if !closureCtx.IsZero() {
		call = append(call, closureCtx)
	}
	if target != nil {
		c.b.TailCall(target, call...)
	} else {
		ft, err := c.calleeType(in, args[0])
		if err != nil {
			return err
		}
		c.b.TailCallInd(through, ft, call...)
	}
	return c.emitResumePoint(s, after)
}

// suspensionTarget is what a suspension calls and how big a context to
// allocate for it: a function this module can name, or -- for a call
// through a value, or on a function another module defines -- the code
// and the size read out of the record beside it.
func (c *fn) suspensionTarget(s *suspension) (ir.Callee, ir.Ptr, ir.Ptr, ir.I64, error) {
	in := s.at
	callee := in.Args()[0]
	none := ir.Ptr{}
	if name, ok := c.refNames[callee]; ok {
		if size, known := c.l.asyncFrameSize(name); known {
			target := c.l.callee[name]
			if target == nil {
				target = c.l.defs[name]
			}
			if target == nil {
				return nil, none, none, ir.I64{}, c.fail(ErrUnsupported, in.Op(),
					"a suspension on a function with no symbol")
			}
			return target, none, none, c.b.I64.Const(size), nil
		}
		// Another module's: the record beside it says both.
		code, size := c.readAsyncRecord(c.b.Ptr.GetAddr(c.l.asyncRecordOf(name)))
		return nil, code, none, size, nil
	}
	// A value. Its first word is the record, not the code -- see
	// thinToThick and partialApply.
	fp, ctx, err := c.calleeValue(in, callee)
	if err != nil {
		return nil, none, none, ir.I64{}, err
	}
	code, size := c.readAsyncRecord(fp)
	return nil, code, ctx, size, nil
}

// readAsyncRecord takes a record apart: the code it names, and the size
// of the context that code wants.
//
//	code = record + sext(record[0])
//	size = zext(record[1])
//
// which is swiftc's own sequence. See asyncRecord.
func (c *fn) readAsyncRecord(at ir.Ptr) (ir.Ptr, ir.I64) {
	off := c.b.I64.SExtI32(c.b.I32.Load(at))
	code := c.b.Ptr.Add(at, off)
	size := c.b.I64.ZExtI32(c.b.I32.Load(c.b.Ptr.Add(at, c.b.I64.Const(asyncRecordSize))))
	return code, size
}

// answers are the values a call comes back with: an apply's results, or
// -- for one that may fail -- the arguments of the edge it takes when it
// does not.
func (c *fn) answers(s *suspension) []*sil.Value {
	if s.throws {
		return s.normal.Args()
	}
	return s.at.Results()
}

// answerStorage is where a call's answer goes when it is too wide for
// registers: the caller sets it aside and the callee writes through it.
func (c *fn) answerStorage(s *suspension) (ir.Ptr, bool) {
	for _, a := range c.answers(s) {
		if slot, ok := c.wide[a]; ok {
			return slot, true
		}
	}
	return ir.Ptr{}, false
}

// emitResumePoint fills the block the body carries on at.
//
// For a call that cannot fail there is nothing to do: the answer is in
// the frame and the instructions after it read it from there. For one
// that can, this is where the two edges part -- the error the stub wrote
// down is null or it is not, which is the whole of how Swift tells a
// failing return from an ordinary one.
func (c *fn) emitResumePoint(s *suspension, after *ir.Block) error {
	saved := c.b
	c.b = after
	defer func() { c.b = saved }()

	// An answer too wide for registers was written into the storage the
	// caller set aside, and from here on that storage is where the value
	// is -- which is what an ordinary call records too. See apply.
	for _, a := range c.answers(s) {
		if slot, ok := c.wide[a]; ok {
			c.mem[a] = slot
		}
	}
	if !s.throws {
		return nil
	}

	failed, ok := c.blocks[s.failed]
	if !ok {
		return c.fail(ErrUnsupported, s.at.Op(), "no block for the edge a failing call takes")
	}
	normal, ok := c.blocks[s.normal]
	if !ok {
		return c.fail(ErrUnsupported, s.at.Op(), "no block for the edge a call takes when it does not fail")
	}
	box, ok := c.loadReg(ir.TypePtr, c.b.Ptr.Add(c.ctx, c.b.I64.Const(s.errSlot))).(ir.Ptr)
	if !ok {
		return c.fail(ErrType, s.at.Op(), "an error that is not a reference")
	}
	carried, err := c.edgeArgs(s)
	if err != nil {
		return err
	}
	threw := c.b.Ptr.Ne(box, c.b.Ptr.Const())
	var errEdge ir.BlockTarget
	if s.errArg != nil {
		errEdge = failed.To(box)
	} else {
		errEdge = failed.To()
	}
	c.b.BrIf(threw, errEdge, normal.To(carried...))
	return nil
}

// edgeArgs is what the edge a call takes when it does not fail carries:
// the answer, in the registers the block it goes to declares.
func (c *fn) edgeArgs(s *suspension) ([]ir.Value, error) {
	var out []ir.Value
	for _, a := range s.normal.Args() {
		if empty(a.Type()) {
			continue
		}
		// One too wide for registers is in storage the callee wrote
		// through, and the block takes it as its scalars.
		if slot, ok := c.wide[a]; ok {
			ls, ok := leavesOf(a.Type())
			if !ok {
				return nil, c.fail(ErrUnsupported, s.at.Op(),
					"a wide answer whose layout this cannot take apart")
			}
			for _, l := range ls {
				r, ok := machineOf(l.typ)
				if !ok {
					return nil, c.fail(ErrType, s.at.Op(), l.typ.String())
				}
				v, err := c.loadScalar(s.at, c.fieldAddr(slot, l.offset), r)
				if err != nil {
					return nil, err
				}
				out = append(out, v)
			}
			continue
		}
		if parts, ok := c.parts(a); ok {
			out = append(out, parts...)
			continue
		}
		got, ok := c.value(a)
		if !ok {
			return nil, c.fail(ErrUnsupported, s.at.Op(), "an answer with nowhere to come back in")
		}
		out = append(out, got)
	}
	return out, nil
}

// asyncFrameSize is how big a context an async function wants, which the
// caller allocates. It is known for a function this module defines,
// because the whole module is in hand before any of it is lowered, and
// not for anything else -- a function another module defines, or one
// reached through a value. Those read the record beside the callee at
// run time instead; see asyncRecord and suspensionTarget.
func (l *lowerer) asyncFrameSize(name string) (int64, bool) {
	// The runtime's suspending primitives keep nothing across a
	// suspension -- they write down where to carry on and return -- so a
	// context of the header and nothing else is the whole of what they
	// want.
	if stdlib.Suspends(name) {
		return int64(stdlib.AsyncContextBytes), true
	}
	size, ok := l.asyncSizes[name]
	return size, ok
}

// The record beside an async function.
//
// A caller of an async function allocates the callee's frame, so it has
// to know how big that frame is -- which is the callee's business and is
// not in its type. Swift answers with a record beside every async
// function, named by the function's symbol and `Tu`:
//
//	<{ i32 relativeFunction, i32 expectedContextSize }>
//
// The first field is the distance from the record to the function, so
// the pair is position independent and can live in read-only memory; the
// second is the size. A call through a function value reads both:
//
//	%3 = load i32, ptr %fp          ; the offset
//	%7 = %fp + sext(%3)             ; the code
//	%9 = load i32, ptr %fp + 4      ; the size
//	%11 = call swiftcc ptr @swift_task_alloc(i64 zext(%9))
//
// which is measured from swiftc rather than guessed. See
// docs/vertex_swift_async.md.

// asyncRecordName is what that record is called for a function.
func asyncRecordName(name string) string { return name + "Tu" }

// asyncRecordOffsets: the function first, the size after it.
const (
	asyncRecordFunc = 0
	asyncRecordSize = 4
)

// asyncRecord places the record for an async function this module
// defines, once, and is it.
func (l *lowerer) asyncRecord(name string, size int64) (*ir.Global, bool) {
	sym := l.sym(asyncRecordName(name))
	if g, ok := l.asyncRecords[sym]; ok {
		return g, true
	}
	fn := l.defs[name]
	if fn == nil {
		return nil, false
	}
	return l.asyncRecordFor(fn, sym, size, true), true
}

// asyncRecordFor places the record for a function this module holds,
// whatever made the function: a declaration, or a closure's forwarder.
//
// A forwarder's size is the body's, not its own -- the caller allocates
// one context and the forwarder passes it straight on rather than making
// a frame of its own.
func (l *lowerer) asyncRecordFor(fn *ir.Func, sym string, size int64, export bool) *ir.Global {
	if g, ok := l.asyncRecords[sym]; ok {
		return g
	}
	g := l.out.Global(sym, ir.RO, ir.Array(2, ir.StoreI32.FType())).Align(8)
	if export {
		// It leaves the object file with the function it belongs to: a
		// caller in another module reads it to size the frame. A
		// closure's does not: nothing outside this object can name the
		// forwarder either.
		g.Export()
	}
	g.Init(ir.List(
		relative(fn, g, asyncRecordFunc),
		ir.Lit(ir.Int(size)),
	))
	if l.asyncRecords == nil {
		l.asyncRecords = map[string]*ir.Global{}
	}
	l.asyncRecords[sym] = g
	return g
}

// asyncRecordOf is the record for an async function, whether this module
// defines it or another does. One it does not define is imported, which
// is how a call across a module boundary sizes the frame it allocates.
func (l *lowerer) asyncRecordOf(name string) ir.Symbol {
	if size, ok := l.asyncSizes[name]; ok {
		if g, made := l.asyncRecord(name, size); made {
			return g
		}
	}
	sym := l.sym(asyncRecordName(name))
	if g, ok := l.asyncImports[sym]; ok {
		return g
	}
	g := l.out.ImportGlobal(sym, ir.Array(2, ir.StoreI32.FType()))
	if l.asyncImports == nil {
		l.asyncImports = map[string]*ir.GlobalImport{}
	}
	l.asyncImports[sym] = g
	return g
}
