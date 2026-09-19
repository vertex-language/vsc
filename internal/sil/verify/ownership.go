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
	for _, v := range f.Values() {
		c.kinds(v)
		switch v.Ownership() {
		case sil.Owned:
			c.lifetime(v, d)
		case sil.Guaranteed:
			c.notConsumed(v)
			c.borrow(v, d)
		}
	}
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

// lifetime validates Rule 1: an owned value is consumed exactly once on all paths.
func (c *collector) lifetime(v *sil.Value, d *domTree) {
	def, _ := definedIn(v)
	if def == nil || !d.Reachable(def) {
		return
	}

	uses := useIndex(v)
	c.twice, c.after = ErrDoubleConsume, ErrUseAfterConsume
	exit := c.walk(v, d, uses, ErrDoubleConsume, ErrUseAfterConsume,
		func(in *sil.Inst, i int) bool { return consumesValue(in, v) })

	// Values left live at non-unreachable block exits are leaks.
	for b, s := range exit {
		if s != live {
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
func (c *collector) borrow(v *sil.Value, d *domTree) {
	in := v.Inst()
	if in == nil || !in.Op().Borrows() {
		return // a @guaranteed parameter's scope is the whole function
	}
	def, _ := definedIn(v)
	if def == nil || !d.Reachable(def) {
		return
	}

	uses := useIndex(v)
	c.twice, c.after = ErrBorrowNotEnded, ErrUseOutsideBorrow
	exit := c.walk(v, d, uses, ErrBorrowNotEnded, ErrUseOutsideBorrow,
		func(use *sil.Inst, i int) bool { return endsBorrow(use) })

	for b, s := range exit {
		if s != live {
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
			if liveAt(v, d, use, exit, uses, func(x *sil.Inst, _ int) bool { return endsBorrow(x) }) {
				c.at(use.Block(), indexOf(use), use.Op(), base[0], ErrUseAfterConsume,
					fmt.Sprintf("consumed while %%%d borrows it", v.ID()))
			}
		}
	}
}

// walk performs a dataflow analysis on the CFG:
// Phase 1 computes exit states until reaching a fixed point.
// Phase 2 reports errors for double-consumption, use-after-consume, or unclosed scopes.
func (c *collector) walk(v *sil.Value, d *domTree, uses map[*sil.Inst]int,
	twice, after error, ends func(*sil.Inst, int) bool) map[*sil.Block]state {

	order := d.order
	exit := map[*sil.Block]state{}

	// Phase one: the fixpoint.
	for changed := true; changed; {
		changed = false
		for _, b := range order {
			s := c.transfer(v, b, mergeStates(d, b, exit), uses, ends, nil)
			if old, seen := exit[b]; !seen || old != s {
				exit[b] = s
				changed = true
			}
		}
	}

	// Phase two: the reporting pass.
	for _, b := range order {
		c.transfer(v, b, mergeStates(d, b, exit), uses, ends, c)
		c.disagreement(v, d, b, exit)
	}
	return exit
}

// transfer executes block transfer functions; report is non-nil during phase 2 reporting.
func (c *collector) transfer(v *sil.Value, b *sil.Block, s state,
	uses map[*sil.Inst]int, ends func(*sil.Inst, int) bool, report *collector) state {

	def, at := definedIn(v)
	// Block arguments are defined afresh when the block is entered.
	if b == def && at < 0 {
		s = live
	}
	for i, in := range b.Insts() {
		// Value is live starting at its definition.
		if b == def && i == at {
			s = live
		}
		if _, isUse := uses[in]; !isUse {
			continue
		}
		switch {
		case ends(in, i):
			if s == consumed && report != nil {
				report.at(b, i, in.Op(), v, report.twice, "")
			}
			s = consumed
		case s == consumed && report != nil:
			report.at(b, i, in.Op(), v, report.after, "")
		}
	}
	return s
}

// disagreement checks for inconsistent states across predecessor paths entering b.
func (c *collector) disagreement(v *sil.Value, d *domTree, b *sil.Block, exit map[*sil.Block]state) {
	var seen bool
	var first state
	for _, p := range d.Preds(b) {
		ps, ok := exit[p]
		if !ok || ps == dead {
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

// mergeStates computes entry state from predecessor exit states.
func mergeStates(d *domTree, b *sil.Block, exit map[*sil.Block]state) state {
	s, seen := dead, false
	for _, p := range d.Preds(b) {
		ps, ok := exit[p]
		if !ok {
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

// useIndex maps each user instruction of v to its index within its block.
func useIndex(v *sil.Value) map[*sil.Inst]int {
	m := make(map[*sil.Inst]int, len(v.Uses()))
	for _, in := range v.Uses() {
		m[in] = indexOf(in)
	}
	return m
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

// liveAt reports whether v is live at instruction in within its block.
func liveAt(v *sil.Value, d *domTree, in *sil.Inst, exit map[*sil.Block]state,
	uses map[*sil.Inst]int, ends func(*sil.Inst, int) bool) bool {

	b := in.Block()
	if b == nil {
		return false
	}
	def, at := definedIn(v)
	s := mergeStates(d, b, exit)

	for i, other := range b.Insts() {
		if b == def && i == at {
			s = live
		}
		if other == in {
			return s == live
		}
		if _, isUse := uses[other]; isUse && ends(other, i) {
			s = consumed
		}
	}
	return false
}

// opOf returns the opcode defining v, or "" for block arguments.
func opOf(v *sil.Value) sil.Op {
	if in := v.Inst(); in != nil {
		return in.Op()
	}
	return ""
}
