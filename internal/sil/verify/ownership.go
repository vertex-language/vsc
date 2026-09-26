package verify

import (
	"fmt"

	"github.com/vertex-language/vsc/internal/sil"
)

// Dataflow states for verifying OSSA lifetime and borrow scopes:
//
//	live      value defined and not yet consumed/ended
//	consumed  value consumed or borrow scope closed
//	dead      value uninitialized/not live

type state uint8

const (
	dead state = iota
	live
	consumed
)

func (s state) String() string {
	switch s {
	case live:
		return "live"
	case consumed:
		return "consumed"
	}
	return "dead"
}

// ownership validates Rule 1 (owned) and Rule 2 (guaranteed) for all values.
func (c *collector) ownership(f *sil.Func, d *domTree) {
	if !f.OSSA() {
		return // ownership was lowered away; the rules no longer apply
	}
	fl := newFlow(f, d)
	for _, v := range f.Values() {
		c.kinds(v)
		switch v.Ownership() {
		case sil.Owned:
			c.lifetime(v, fl)
		case sil.Guaranteed:
			c.notConsumed(v)
			c.borrow(v, fl)
		}
	}
}

// A flow is what the per-value dataflow of one function shares: where
// every instruction is, and each reachable block's place in reverse
// postorder, which indexes the per-value states.
//
// Each value's walk visits only the blocks that define or use it. A
// block that does neither hands its entry state to its exit unchanged,
// so the answer is the one a walk over every instruction of every block
// gives, without the cost that walk has per value.
type flow struct {
	d     *domTree
	pos   map[*sil.Inst]int // an instruction's index in its block
	preds [][]int           // each reachable block's reachable predecessors, by rpo index
	exit  []state           // scratch: each block's exit state for the value being walked
	busy  []bool            // scratch: blocks that define or use it
}

// unset marks a block the fixpoint has not reached.
const unset state = 0xff

func newFlow(f *sil.Func, d *domTree) *flow {
	fl := &flow{d: d, pos: map[*sil.Inst]int{}}
	for _, b := range f.Blocks() {
		for i, in := range b.Insts() {
			fl.pos[in] = i
		}
	}
	fl.preds = make([][]int, len(d.order))
	for i, b := range d.order {
		for _, p := range d.Preds(b) {
			if k, ok := d.rpo[p]; ok {
				fl.preds[i] = append(fl.preds[i], k)
			}
		}
	}
	fl.exit = make([]state, len(d.order))
	fl.busy = make([]bool, len(d.order))
	return fl
}

// a value's def site and uses, as the walk needs them.
type valueFlow struct {
	v     *sil.Value
	def   *sil.Block
	at    int
	uses  map[*sil.Inst]int
	ends  func(*sil.Inst, int) bool
	touch []int // rpo indices of the blocks that define or use v
}

func (fl *flow) prepare(v *sil.Value, ends func(*sil.Inst, int) bool) *valueFlow {
	def, _ := definedIn(v)
	at := -1
	if in := v.Inst(); in != nil {
		at = fl.pos[in]
	}
	vf := &valueFlow{v: v, def: def, at: at, uses: make(map[*sil.Inst]int, len(v.Uses())), ends: ends}
	mark := func(b *sil.Block) {
		if k, ok := fl.d.rpo[b]; ok && !fl.busy[k] {
			fl.busy[k] = true
			vf.touch = append(vf.touch, k)
		}
	}
	if def != nil {
		mark(def)
	}
	for _, in := range v.Uses() {
		vf.uses[in] = fl.pos[in]
		mark(in.Block())
	}
	for _, k := range vf.touch {
		fl.busy[k] = false
	}
	return vf
}

// lifetime validates Rule 1: an owned value is consumed exactly once on all paths.
func (c *collector) lifetime(v *sil.Value, fl *flow) {
	def, _ := definedIn(v)
	if def == nil || !fl.d.Reachable(def) {
		return
	}
	vf := fl.prepare(v, func(in *sil.Inst, i int) bool { return consumesValue(in, v) })
	c.twice, c.after = ErrDoubleConsume, ErrUseAfterConsume
	exit := c.walk(vf, fl)

	// Values left live at non-unreachable block exits are leaks.
	for k, b := range fl.d.order {
		if exit[k] != live {
			continue
		}
		t := b.Term()
		if t == nil || len(t.Successors()) > 0 || t.Op() == sil.Unreachable {
			continue
		}
		c.at(b, indexOf(t), t.Op(), v, ErrLeak, "")
	}
}

// borrow validates Rule 2: guaranteed values must only be used within their borrow scope.
func (c *collector) borrow(v *sil.Value, fl *flow) {
	in := v.Inst()
	if in == nil || !in.Op().Borrows() {
		return // a @guaranteed parameter's scope is the whole function
	}
	def, _ := definedIn(v)
	if def == nil || !fl.d.Reachable(def) {
		return
	}
	vf := fl.prepare(v, func(use *sil.Inst, i int) bool { return endsBorrow(use) })
	c.twice, c.after = ErrBorrowNotEnded, ErrUseOutsideBorrow
	exit := c.walk(vf, fl)

	for k, b := range fl.d.order {
		if exit[k] != live {
			continue
		}
		t := b.Term()
		if t == nil || len(t.Successors()) > 0 || t.Op() == sil.Unreachable {
			continue
		}
		c.at(b, indexOf(t), t.Op(), v, ErrBorrowNotEnded, "")
	}

	// The borrowed base value cannot be consumed while borrow is active.
	if base := in.Args(); len(base) == 1 && base[0] != nil {
		for _, use := range base[0].Consumers() {
			if fl.liveAt(vf, exit, use) {
				c.at(use.Block(), indexOf(use), use.Op(), base[0], ErrUseAfterConsume,
					fmt.Sprintf("consumed while %%%d borrows it", v.ID()))
			}
		}
	}
}

// walk performs a dataflow analysis on the CFG:
// Phase 1 computes exit states until reaching a fixed point.
// Phase 2 reports errors for double-consumption, use-after-consume, or unclosed scopes.
//
// The states it returns are the flow's scratch, indexed by reverse
// postorder, and good until the next walk.
func (c *collector) walk(vf *valueFlow, fl *flow) []state {
	exit := fl.exit
	for k := range exit {
		exit[k] = unset
	}
	for _, k := range vf.touch {
		fl.busy[k] = true
	}
	defer func() {
		for _, k := range vf.touch {
			fl.busy[k] = false
		}
	}()

	// Phase one: the fixpoint.
	for changed := true; changed; {
		changed = false
		for k, b := range fl.d.order {
			s := fl.merge(k, exit)
			if fl.busy[k] {
				s = c.transfer(vf, b, s, nil)
			}
			if exit[k] != s {
				exit[k] = s
				changed = true
			}
		}
	}

	// Phase two: the reporting pass.
	for k, b := range fl.d.order {
		if fl.busy[k] {
			c.transfer(vf, b, fl.merge(k, exit), c)
		}
		c.disagreement(vf.v, fl, k, b, exit)
	}
	return exit
}

// transfer executes block transfer functions; report is non-nil during phase 2 reporting.
func (c *collector) transfer(vf *valueFlow, b *sil.Block, s state, report *collector) state {
	// Block arguments are defined afresh when the block is entered.
	if b == vf.def && vf.at < 0 {
		s = live
	}
	for i, in := range b.Insts() {
		// Value is live starting at its definition.
		if b == vf.def && i == vf.at {
			s = live
		}
		if _, isUse := vf.uses[in]; !isUse {
			continue
		}
		switch {
		case vf.ends(in, i):
			if s == consumed && report != nil {
				report.at(b, i, in.Op(), vf.v, report.twice, "")
			}
			s = consumed
		case s == consumed && report != nil:
			report.at(b, i, in.Op(), vf.v, report.after, "")
		}
	}
	return s
}

// disagreement checks for inconsistent states across predecessor paths entering b.
func (c *collector) disagreement(v *sil.Value, fl *flow, k int, b *sil.Block, exit []state) {
	var seen bool
	var first state
	for _, p := range fl.preds[k] {
		ps := exit[p]
		if ps == unset || ps == dead {
			continue
		}
		if !seen {
			first, seen = ps, true
			continue
		}
		if ps != first {
			c.at(b, -1, "", v, ErrLeak,
				"live on one path into "+b.Label()+" and consumed on another")
			return
		}
	}
}

// merge computes a block's entry state from its predecessors' exit states.
func (fl *flow) merge(k int, exit []state) state {
	s, seen := dead, false
	for _, p := range fl.preds[k] {
		ps := exit[p]
		if ps == unset {
			continue
		}
		if !seen {
			s, seen = ps, true
			continue
		}
		if ps == consumed || s == consumed {
			s = consumed
		}
	}
	return s
}

// liveAt reports whether the walked value is live at instruction in.
func (fl *flow) liveAt(vf *valueFlow, exit []state, in *sil.Inst) bool {
	b := in.Block()
	if b == nil {
		return false
	}
	k, ok := fl.d.rpo[b]
	s := dead
	if ok {
		s = fl.merge(k, exit)
	}
	for i, other := range b.Insts() {
		if b == vf.def && i == vf.at {
			s = live
		}
		if other == in {
			return s == live
		}
		if _, isUse := vf.uses[other]; isUse && vf.ends(other, i) {
			s = consumed
		}
	}
	return false
}

// kinds validates ownership kinds against value types (e.g. trivial types must have OwnershipNone).
func (c *collector) kinds(v *sil.Value) {
	if v.Type().Trivial() && v.Ownership() != sil.None {
		b, i := definedIn(v)
		c.at(b, i, opOf(v), v, ErrOwnership,
			"a value of a trivial type cannot be "+v.Ownership().String())
	}
	if v.Ownership() != sil.None {
		return
	}
	// Nothing that owns nothing may be destroyed or borrowed.
	for _, use := range v.Uses() {
		switch use.Op() {
		case sil.DestroyValue, sil.BeginBorrow, sil.CopyValue:
			if v.Type().Trivial() {
				c.at(use.Block(), indexOf(use), use.Op(), v, ErrOwnership,
					"the operand owns nothing")
			}
		}
	}
}

// notConsumed checks that guaranteed (borrowed) values are not consumed.
func (c *collector) notConsumed(v *sil.Value) {
	for _, use := range v.Consumers() {
		c.at(use.Block(), indexOf(use), use.Op(), v, ErrConsumedGuaranteed, "")
	}
}

// consumesValue reports whether instruction in takes ownership of operand v.
func consumesValue(in *sil.Inst, v *sil.Value) bool {
	// A partial application's captured values belong to the context it
	// makes, whatever the callee's parameters say: it consumes them.
	if in.Op() == sil.PartialApply {
		for i, a := range in.Args() {
			if i > 0 && a == v {
				return true
			}
		}
		return false
	}
	switch in.Op() {
	case sil.Apply, sil.TryApply:
		callee := calleeType(in)
		for i, a := range in.Args() {
			if a == v && sil.ConsumesArgument(callee, i) {
				return true
			}
		}
		return false
	}
	for i, a := range in.Args() {
		if a == v && in.Op().Consumes(i) {
			return true
		}
	}
	return false
}

// calleeType is the lowered function type a call is calling.
func calleeType(in *sil.Inst) *sil.FuncType {
	args := in.Args()
	if len(args) == 0 || args[0] == nil {
		return nil
	}
	f, _ := args[0].Type().Formal().(*sil.FuncType)
	return f
}

// endsBorrow reports whether an instruction closes a borrow scope.
func endsBorrow(in *sil.Inst) bool {
	return in.Op() == sil.EndBorrow || in.Op() == sil.EndAccess
}

// opOf returns the opcode defining v, or "" for block arguments.
func opOf(v *sil.Value) sil.Op {
	if in := v.Inst(); in != nil {
		return in.Op()
	}
	return ""
}
