package lower

import (
	"github.com/vertex-language/vsc/internal/sil"
	"github.com/vertex-language/vsc/stdlib"
	"github.com/vertex-language/vsc/types"
)

// Splitting an async function.
//
// An async function has no stack: a suspended one has given its thread
// back, so anything that has to outlive a suspension lives in a frame the
// task allocator owns, reached through the context register.
//
// That makes the function several functions. Control leaves at each
// suspension by tail calling the callee, and comes back at a funclet the
// callee's context names, so each stretch between suspensions is compiled
// as a function of its own. This file works out where the cuts fall and
// what has to cross them; asyncsplit.go does the cutting.
//
// The protocol is Swift's, read off what swiftc emits rather than
// invented. A call is:
//
//	callee := task_alloc(calleeFrameSize)
//	callee->parent       = myContext
//	callee->resumeParent = <the funclet to come back to>
//	tail_call f(callee, args…)
//
// and the callee returns by tail calling `callee->resumeParent(callee,
// results…)`. So a funclet is entered with the *callee's* context, finds
// its own at the callee's first word, and frees the callee's frame.
//
// A callee that may fail answers the same way, with the error last, in
// the self register, and null there where it did not. That is Swift's
// too, and it is why a try_apply of an async callee is a suspension like
// any other: the two edges part at the funclet, on whether the error is
// null.
//
// calleeFrameSize is worked out here for a function this module defines
// and read at run time for anything else -- another module's, or a
// function value's -- out of the record beside the callee. See
// asyncsplit.go.
//
// See docs/vertex_swift_async.md for the measurements this rests on.

// A suspension is one point where an async function gives up its thread:
// a call to another async function, whether or not it may fail.
type suspension struct {
	// at is the call, in the block it appears in.
	at  *sil.Inst
	blk *sil.Block
	// index is which suspension this is, counting from zero, which is
	// what the funclet's name is built from.
	index int
	// live are the values defined before this suspension and used after
	// it, in the order they are laid out in the frame.
	live []*sil.Value
	// calleeSlot is where the callee's context is kept while it runs,
	// so the funclet can free it.
	calleeSlot int64
	// results are the values the call answers with. The resume stub is
	// handed them in registers and writes them here; the body reads them
	// from here, because it is entered without them.
	results []*sil.Value
	// resultWords is how many registers the answer arrives in.
	resultWords int
	// throws is whether the call may fail, which is a try_apply rather
	// than an apply. The error comes back with the results, in the self
	// register, and is null where nothing was thrown -- see
	// lowerer.continuationType.
	throws bool
	// errSlot is where that error is kept between the stub writing it
	// down and the body branching on it.
	errSlot int64
	// normal and failed are the edges a try_apply leaves by, and errArg
	// is the block argument the failed edge carries the error in.
	normal, failed *sil.Block
	errArg         *sil.Value
}

// An asyncPlan is what splitting one function needs to know: where it
// gives up its thread, what has to survive each time, and how big a frame
// that comes to.
type asyncPlan struct {
	suspends []*suspension
	// slot is where a value lives in the frame, for every value that
	// crosses any suspension. Offsets are from the start of the frame,
	// which is the context plus its header.
	slot map[*sil.Value]int64
	// storage is where a piece of memory the body wants lives, for
	// every piece of it: an alloc_stack, a wide value held by address.
	// They are in the frame rather than on the machine stack because
	// the machine stack does not survive a suspension. See storageWalk.
	storage map[*sil.Value]int64
	// kind is what each of those pieces is for.
	kind map[*sil.Value]storageKind
	// size is the whole context: the header and the frame after it.
	size int64
	// params are the entry block's arguments, which the prologue writes
	// into the frame for the body to read.
	params []*sil.Value
	// sret is where the pointer to the caller's result storage is kept,
	// for a function whose result comes back by address. It is a
	// parameter like any other, and crosses like one; it is not a SIL
	// value, so it has a place of its own rather than a slot.
	sret    int64
	hasSRet bool
}

// suspensionAt is the suspension an instruction is, or nil.
func (p *asyncPlan) suspensionAt(in *sil.Inst) *suspension {
	if p == nil {
		return nil
	}
	for _, s := range p.suspends {
		if s.at == in {
			return s
		}
	}
	return nil
}

// suspends reports whether the function gives up its thread at all. One
// that does not still returns through its continuation, but it needs no
// frame and no funclets.
func (p *asyncPlan) hasSuspensions() bool { return p != nil && len(p.suspends) > 0 }

// planAsync works out how to split f, or nil where f is not async.
func planAsync(l *lowerer, f *sil.Func) (*asyncPlan, error) {
	if f == nil || f.Type() == nil || !f.Type().Async {
		return nil, nil
	}
	// A declaration has no body to split and no frame this module can
	// work out. Its size comes from the record beside it, which its own
	// module placed -- see asyncRecordOf. Answering with the header
	// here would be a guess, and a wrong one for anything that
	// suspends.
	if f.IsDeclaration() {
		return nil, nil
	}
	p := &asyncPlan{
		slot:    map[*sil.Value]int64{},
		storage: map[*sil.Value]int64{},
		kind:    map[*sil.Value]storageKind{},
	}
	for _, b := range f.Blocks() {
		for _, in := range b.Insts() {
			if !isSuspension(in) {
				continue
			}
			s := &suspension{at: in, blk: b, index: len(p.suspends)}
			if in.Op() == sil.TryApply {
				s.throws = true
				for _, k := range in.Aux().Cases {
					switch k.Member {
					case "normal":
						s.normal = k.Dest
					case "error":
						s.failed = k.Dest
						if len(k.Dest.Args()) == 1 {
							s.errArg = k.Dest.Args()[0]
						}
					}
				}
				if s.normal == nil || s.failed == nil {
					return nil, &Error{Err: ErrUnsupported, Func: f.SourceName(),
						What: "an await on a call that may fail, with fewer than both edges"}
				}
			}
			p.suspends = append(p.suspends, s)
		}
	}
	if len(p.suspends) == 0 {
		p.size = int64(stdlib.AsyncContextBytes)
		return p, nil
	}
	liveAcross(f, p)
	if err := p.layout(l, f); err != nil {
		return nil, err
	}
	return p, nil
}

// isSuspension reports whether an instruction gives up the thread: a
// call of a callee whose type is async, whether or not it may fail.
//
// A call to an ordinary function is not one however long it takes. That
// is the rule that makes `await` mean something -- see SE-0296, and
// docs/vertex_swift_async.md.
func isSuspension(in *sil.Inst) bool {
	if in == nil || (in.Op() != sil.Apply && in.Op() != sil.TryApply) {
		return false
	}
	args := in.Args()
	if len(args) == 0 || args[0] == nil {
		return false
	}
	return isAsyncCallee(args[0].Type())
}

// isAsyncCallee reports whether a callee -- a named function, or a
// function value -- is async.
func isAsyncCallee(t sil.Type) bool {
	f := t.Formal()
	if f == nil {
		return false
	}
	if ft, ok := f.(*sil.FuncType); ok {
		return ft.Async
	}
	// A value's type is the Swift signature, not SIL's function type.
	sig, ok := f.Underlying().(*types.Signature)
	return ok && sig.Async
}

// liveAcross fills in each suspension's live set: the values defined
// before it that are still wanted after it.
//
// It is ordinary backward liveness over the blocks, with one addition --
// the answer is asked for at each suspension rather than only at block
// boundaries, because that is where the frame has to hold something.
func liveAcross(f *sil.Func, p *asyncPlan) {
	// liveOut[b] is what is live leaving b; liveIn[b] entering it.
	liveIn := map[*sil.Block]map[*sil.Value]bool{}
	liveOut := map[*sil.Block]map[*sil.Value]bool{}
	for _, b := range f.Blocks() {
		liveIn[b] = map[*sil.Value]bool{}
		liveOut[b] = map[*sil.Value]bool{}
	}

	// To a fixed point. The CFG is small and this is not a hot path.
	for changed := true; changed; {
		changed = false
		blocks := f.Blocks()
		for i := len(blocks) - 1; i >= 0; i-- {
			b := blocks[i]
			out := map[*sil.Value]bool{}
			if t := b.Term(); t != nil {
				for _, s := range t.Successors() {
					for v := range liveIn[s] {
						out[v] = true
					}
				}
			}
			in := map[*sil.Value]bool{}
			for v := range out {
				in[v] = true
			}
			// Backwards through the block: a result kills, a use gives life.
			insts := b.Insts()
			for j := len(insts) - 1; j >= 0; j-- {
				for _, r := range insts[j].Results() {
					delete(in, r)
				}
				for _, a := range insts[j].Args() {
					if a != nil {
						in[a] = true
					}
				}
			}
			// A block's own arguments are defined by it.
			for _, a := range b.Args() {
				delete(in, a)
			}
			if !sameSet(in, liveIn[b]) || !sameSet(out, liveOut[b]) {
				liveIn[b], liveOut[b] = in, out
				changed = true
			}
		}
	}

	// Now walk each block forwards, carrying the live set, and read it
	// off at every suspension.
	for _, b := range f.Blocks() {
		// What is live at the end of the block, walked backwards to the
		// point of each suspension.
		live := map[*sil.Value]bool{}
		for v := range liveOut[b] {
			live[v] = true
		}
		insts := b.Insts()
		at := map[*sil.Inst]map[*sil.Value]bool{}
		for j := len(insts) - 1; j >= 0; j-- {
			in := insts[j]
			for _, r := range in.Results() {
				delete(live, r)
			}
			if isSuspension(in) {
				// What is live here is what the funclet after this
				// suspension will want. The apply's own result is
				// excluded: it arrives as the funclet's argument.
				snap := map[*sil.Value]bool{}
				for v := range live {
					snap[v] = true
				}
				at[in] = snap
			}
			for _, a := range in.Args() {
				if a != nil {
					live[a] = true
				}
			}
		}
		for _, s := range p.suspends {
			if s.blk != b {
				continue
			}
			s.live = ordered(f, at[s.at])
		}
	}
}

// ordered puts a live set in a stable order, so that a frame's layout
// does not depend on map iteration.
func ordered(f *sil.Func, set map[*sil.Value]bool) []*sil.Value {
	var out []*sil.Value
	for _, b := range f.Blocks() {
		for _, a := range b.Args() {
			if set[a] {
				out = append(out, a)
			}
		}
		for _, in := range b.Insts() {
			for _, r := range in.Results() {
				if set[r] {
					out = append(out, r)
				}
			}
		}
	}
	return out
}

func sameSet(a, b map[*sil.Value]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for v := range a {
		if !b[v] {
			return false
		}
	}
	return true
}

// layout gives a slot to everything that has to cross a boundary, and
// says how big the context comes to.
//
// Four things cross. The state, which says where to carry on. The
// function's parameters, because the body is entered with a context and
// nothing else. Whatever is live across a suspension. And what each call
// answers with, which arrives at a resume stub in registers and is read
// by the body.
//
// One slot per value over the whole function rather than per suspension:
// a value live across two of them is in one place, so the pieces agree
// about where it is without anything being copied between them.
func (p *asyncPlan) layout(l *lowerer, f *sil.Func) error {
	// The state first, at a fixed place, so that reading it needs
	// nothing else worked out.
	at := int64(stdlib.AsyncContextBytes) + 8

	// Everything the body would have put on its machine stack. It goes
	// first because what comes after is decided by what is left: a
	// value whose storage is here is reached by address and needs no
	// slot of its own.
	probe := &fn{l: l, src: f}
	err := probe.storageWalk(func(k storageKind, v *sil.Value, size, align int64) error {
		if v == nil || size <= 0 {
			return nil
		}
		if align < 8 {
			align = 8
		}
		at = roundUp(at, align)
		p.storage[v] = at
		p.kind[v] = k
		at += size
		return nil
	})
	if err != nil {
		return err
	}

	give := func(v *sil.Value) {
		if v == nil || empty(v.Type()) {
			return
		}
		// A value whose storage is in the frame is reached by address,
		// and that address is the same at every resume, so there is
		// nothing to carry across.
		if _, held := p.storage[v]; held {
			return
		}
		if _, seen := p.slot[v]; seen {
			return
		}
		at = roundUp(at, 8)
		p.slot[v] = at
		at += int64(8 * valueWords(v))
	}

	// The storage the caller set aside for a result too wide to return
	// in registers, before the parameters, because that is the order it
	// arrives in.
	if probe.returnsIndirectly() {
		p.sret, p.hasSRet = at, true
		at += 8
	}

	// The parameters, in order.
	if blocks := f.Blocks(); len(blocks) > 0 {
		for _, a := range blocks[0].Args() {
			give(a)
			p.params = append(p.params, a)
		}
	}
	for _, s := range p.suspends {
		for _, v := range s.live {
			give(v)
		}
		// What the call answers with. For a call that may fail those are
		// the arguments of the edge it takes when it does not: the SIL
		// is a try_apply, and its answer arrives at a block rather than
		// as a result.
		answers := s.at.Results()
		if s.throws {
			answers = s.normal.Args()
		}
		for _, r := range answers {
			if empty(r.Type()) {
				continue
			}
			// One too wide for registers never was in them: the caller
			// set storage aside and the callee wrote through it, and
			// that storage is in this frame already.
			if _, held := p.storage[r]; held {
				continue
			}
			give(r)
			s.results = append(s.results, r)
			s.resultWords += valueWords(r)
		}
		if s.throws {
			at = roundUp(at, 8)
			s.errSlot = at
			at += 8
		}
	}
	// The callee's context, one per suspension: a stub frees the frame
	// of the call it came back from, and needs its address.
	for _, s := range p.suspends {
		at = roundUp(at, 8)
		s.calleeSlot = at
		at += 8
	}
	p.size = roundUp(at, 16)
	return nil
}

// valueWords is how many registers a value travels in, and so how many
// words it takes in the frame. See frameRegs, which decides both.
func valueWords(v *sil.Value) int {
	if v == nil {
		return 0
	}
	rs, ok := frameRegs(v.Type())
	if !ok {
		return 1
	}
	return len(rs)
}

// frameSlot is how much room a value takes in the frame, and what it has
// to be aligned to.
func frameSlot(v *sil.Value) (size, align int64) {
	return int64(8 * valueWords(v)), 8
}

func roundUp(n, to int64) int64 {
	if to <= 1 {
		return n
	}
	return (n + to - 1) / to * to
}
