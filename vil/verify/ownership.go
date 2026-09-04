package verify

import (
	"fmt"

	"github.com/vertex-language/vsc/vil"
)

// The two rules.
//
//  1. An owned value is consumed exactly once on every path out of
//     its definition.
//  2. A guaranteed value is used only within the borrow scope that
//     produced it.
//
// Both are statements about paths, so both are dataflow. The state a
// value is in at a program point is one of three, and the whole check
// is what each instruction does to it and what happens where paths
// meet:
//
//	live      it exists and something owes it a consume
//	consumed  it has been given away; using it now is a use-after
//	dead      it was never live here
//
// Where two paths meet with different states, the value is consumed
// on one and not the other, which is a mistake in itself and the one
// Swift reports as consuming in a branch.

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

// ownership checks every value in the function against both rules.
func (c *collector) ownership(f *vil.Func, d *domTree) {
	if !f.OSSA() {
		return // ownership was lowered away; the rules no longer apply
	}
	for _, v := range f.Values() {
		c.kinds(v)
		switch v.Ownership() {
		case vil.Owned:
			c.lifetime(v, d)
		case vil.Guaranteed:
			c.notConsumed(v)
			c.borrow(v, d)
		}
	}
}

// kinds checks a value's ownership against what it could possibly
// have: a trivial type owns nothing, and nothing owns a trivial type.
func (c *collector) kinds(v *vil.Value) {
	if v.Type().Trivial() && v.Ownership() != vil.None {
		b, i := definedIn(v)
		c.at(b, i, opOf(v), v, ErrOwnership,
			"a value of a trivial type cannot be "+v.Ownership().String())
	}
	if v.Ownership() != vil.None {
		return
	}
	// Nothing that owns nothing may be destroyed or borrowed.
	for _, use := range v.Uses() {
		switch use.Op() {
		case vil.DestroyValue, vil.BeginBorrow, vil.CopyValue:
			if v.Type().Trivial() {
				c.at(use.Block(), indexOf(use), use.Op(), v, ErrOwnership,
					"the operand owns nothing")
			}
		}
	}
}

// notConsumed is rule two's other half: a borrowed value belongs to
// someone else, so nothing may give it away. Returning it as owned,
// destroying it, or passing it where an @owned parameter is declared
// are all the same mistake.
func (c *collector) notConsumed(v *vil.Value) {
	for _, use := range v.Consumers() {
		c.at(use.Block(), indexOf(use), use.Op(), v, ErrConsumedGuaranteed, "")
	}
}

// lifetime is rule one. It walks the function once, carrying the
// value's state, and reports the three ways a lifetime can be wrong:
// consumed twice, used after being consumed, and still live where the
// function ends.
func (c *collector) lifetime(v *vil.Value, d *domTree) {
	def, _ := definedIn(v)
	if def == nil || !d.Reachable(def) {
		return
	}

	uses := useIndex(v)
	c.twice, c.after = ErrDoubleConsume, ErrUseAfterConsume
	exit := c.walk(v, d, uses, ErrDoubleConsume, ErrUseAfterConsume,
		func(in *vil.Inst, i int) bool { return consumesValue(in, v) })

	// What is left live where the function ends is a leak. An
	// unreachable terminator is the exception Swift makes too: a path
	// that does not return does not have to tidy up.
	for b, s := range exit {
		if s != live {
			continue
		}
		t := b.Term()
		if t == nil || len(t.Successors()) > 0 || t.Op() == vil.Unreachable {
			continue
		}
		c.at(b, indexOf(t), t.Op(), v, ErrLeak, "")
	}
}

// borrow is rule two. A borrow's scope is opened by the instruction
// that produced it and closed by end_borrow or end_access; every path
// must close it, and no use may fall outside it.
func (c *collector) borrow(v *vil.Value, d *domTree) {
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
		func(use *vil.Inst, i int) bool { return endsBorrow(use) })

	for b, s := range exit {
		if s != live {
			continue
		}
		t := b.Term()
		if t == nil || len(t.Successors()) > 0 || t.Op() == vil.Unreachable {
			continue
		}
		c.at(b, indexOf(t), t.Op(), v, ErrBorrowNotEnded, "")
	}

	// The value a borrow was taken from must outlive it: it may not
	// be consumed while the borrow is still open.
	if base := in.Args(); len(base) == 1 && base[0] != nil {
		for _, use := range base[0].Consumers() {
			if liveAt(v, use, exit, uses, func(x *vil.Inst, _ int) bool { return endsBorrow(x) }) {
				c.at(use.Block(), indexOf(use), use.Op(), base[0], ErrUseAfterConsume,
					fmt.Sprintf("consumed while %%%d borrows it", v.ID()))
			}
		}
	}
}

// walk carries a value's state through the function and reports what
// happens to it. ends says which instruction ends the value's
// lifetime — a consume for rule one, an end_borrow for rule two — and
// the two sentinels name what a second one and a later use are.
//
// Two phases, and the order matters. The first computes the state at
// every block's exit and says nothing: a loop is walked until the
// answer stops changing, and a check that reported during that would
// report the same fault once per round. The second walks each block
// once, with the answer already known, and is the only phase that
// speaks.
func (c *collector) walk(v *vil.Value, d *domTree, uses map[*vil.Inst]int,
	twice, after error, ends func(*vil.Inst, int) bool) map[*vil.Block]state {

	order := reversePostorder(v.Func())
	exit := map[*vil.Block]state{}

	// Phase one: the fixpoint.
	for changed := true; changed; {
		changed = false
		for _, b := range order {
			s := c.transfer(v, b, mergeStates(b, exit), uses, ends, nil)
			if old, seen := exit[b]; !seen || old != s {
				exit[b] = s
				changed = true
			}
		}
	}

	// Phase two: the reporting pass.
	for _, b := range order {
		c.transfer(v, b, mergeStates(b, exit), uses, ends, c)
		c.disagreement(v, b, exit)
	}
	return exit
}

// transfer is what one block does to a value's state. report is nil
// during the fixpoint and the collector during the reporting pass,
// which is the whole difference between the two.
func (c *collector) transfer(v *vil.Value, b *vil.Block, s state,
	uses map[*vil.Inst]int, ends func(*vil.Inst, int) bool, report *collector) state {

	def, at := definedIn(v)
	for i, in := range b.Insts() {
		// The value comes into existence at its definition, not at
		// the top of the block that holds it.
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
	if b == def && at < 0 {
		// A block argument is defined before every instruction, so a
		// block that only declares it leaves it live.
		if s == dead {
			s = live
		}
	}
	return s
}

// disagreement reports a block two paths reach with the value in
// different states: consumed on one and live on the other, which is
// the mistake Swift reports as consuming in a branch.
func (c *collector) disagreement(v *vil.Value, b *vil.Block, exit map[*vil.Block]state) {
	var seen bool
	var first state
	for _, p := range b.Preds() {
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

// mergeStates is a block's entry state: what its predecessors left.
// Where they disagree the consumed state wins, so that a use after
// the merge is reported once, at the use.
func mergeStates(b *vil.Block, exit map[*vil.Block]state) state {
	s, seen := dead, false
	for _, p := range b.Preds() {
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

// useIndex is the set of instructions that use a value, which is what
// the walk consults rather than scanning every operand list again.
func useIndex(v *vil.Value) map[*vil.Inst]int {
	m := make(map[*vil.Inst]int, len(v.Uses()))
	for _, in := range v.Uses() {
		m[in] = indexOf(in)
	}
	return m
}

// consumesValue reports whether an instruction takes ownership of
// this particular operand — which position it is in decides, since
// store consumes its value and not its address.
//
// A call is asked differently. What an apply does with an argument is
// not in its opcode but in the callee's convention, so the callee's
// type is what answers.
func consumesValue(in *vil.Inst, v *vil.Value) bool {
	switch in.Op() {
	case vil.Apply, vil.TryApply, vil.PartialApply:
		callee := calleeType(in)
		for i, a := range in.Args() {
			if a == v && vil.ConsumesArgument(callee, i) {
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
func calleeType(in *vil.Inst) *vil.FuncType {
	args := in.Args()
	if len(args) == 0 || args[0] == nil {
		return nil
	}
	f, _ := args[0].Type().Formal().(*vil.FuncType)
	return f
}

// endsBorrow reports whether an instruction closes a borrow scope.
func endsBorrow(in *vil.Inst) bool {
	return in.Op() == vil.EndBorrow || in.Op() == vil.EndAccess
}

// liveAt reports whether v is still live where in runs.
//
// A block's exit state is not enough to answer this: a borrow opened
// and closed in one block is live in the middle of it and dead at the
// end, and the consume this catches sits in the middle. So the block
// is walked to the instruction, carrying the state it was entered
// with.
func liveAt(v *vil.Value, in *vil.Inst, exit map[*vil.Block]state,
	uses map[*vil.Inst]int, ends func(*vil.Inst, int) bool) bool {

	b := in.Block()
	if b == nil {
		return false
	}
	def, at := definedIn(v)
	s := mergeStates(b, exit)

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

// opOf is the opcode that defined a value, or the zero Op for a block
// argument.
func opOf(v *vil.Value) vil.Op {
	if in := v.Inst(); in != nil {
		return in.Op()
	}
	return ""
}
