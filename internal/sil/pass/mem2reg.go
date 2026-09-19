package pass

import (
	"github.com/vertex-language/vsc/internal/sil"
)

// Stack-slot promotion: swiftc's SILMem2Reg, for the slots it matters most
// for.
//
// A `var` of trivial type -- an Int, a Bool, a UInt8, a pointer -- is an
// alloc_stack that every read loads from and every write stores to, and
// lowered as it stands each of those is a memory access: a loop counter
// costs a load and a store per iteration. When nothing takes the slot's
// address beyond loading and storing through it, the slot is only a
// variable, and the loads can read the value last stored instead. Where
// two stores reach one load, the block the paths meet at takes the value
// as a block argument, which is SIL's phi.
//
// This is the textbook construction (Cytron et al.): block arguments at the
// iterated dominance frontier of the stores, pruned to the blocks the slot
// is live into, then a walk of the dominator tree that carries the current
// value down it. Only trivial slots are promoted -- their loads and stores
// say [trivial] -- so there is no ownership to thread through the new
// arguments; a slot whose value would need a retain on each copy is left in
// memory, as are slots whose address goes anywhere but a load, a store, an
// access scope or its own dealloc_stack.
//
// A slot is promoted only when every read of it has a value to read on
// every path, which definite initialization already guarantees for a
// source `var`; a slot the walk finds read with nothing stored is left
// alone rather than given an undefined value.
func promoteSlots(f *sil.Func) {
	if f.IsDeclaration() {
		return
	}
	var slots []*sil.Inst
	for _, b := range f.Blocks() {
		for _, in := range b.Insts() {
			if in.Op() == sil.AllocStack {
				slots = append(slots, in)
			}
		}
	}
	if len(slots) == 0 {
		return
	}
	g := newDomInfo(f)
	for _, alloc := range slots {
		acc, ok := slotAccesses(alloc)
		if !ok {
			continue
		}
		plan, ok := g.planSlot(alloc, acc)
		if !ok {
			continue
		}
		applySlot(alloc, acc, plan)
	}
}

// accesses is every instruction that touches a promotable slot.
type accesses struct {
	loads, stores []*sil.Inst
	scopes        []*sil.Inst // begin_access
	ends          []*sil.Inst // end_access, dealloc_stack, debug_value
	of            map[*sil.Inst]bool
}

// slotAccesses collects the uses of alloc, reporting false when one of
// them is anything a promotion cannot account for.
func slotAccesses(alloc *sil.Inst) (accesses, bool) {
	acc := accesses{of: map[*sil.Inst]bool{}}
	addr := alloc.Result()
	if addr == nil {
		return acc, false
	}
	var visit func(v *sil.Value, scoped bool) bool
	visit = func(v *sil.Value, scoped bool) bool {
		for _, use := range v.Uses() {
			switch use.Op() {
			case sil.Load:
				if !onlyAttr(use, "trivial") || use.Args()[0] != v {
					return false
				}
				acc.loads = append(acc.loads, use)
			case sil.Store:
				if !onlyAttr(use, "trivial") || use.Args()[1] != v || use.Args()[0] == v {
					return false
				}
				acc.stores = append(acc.stores, use)
			case sil.BeginAccess:
				if scoped {
					return false
				}
				acc.scopes = append(acc.scopes, use)
				if !visit(use.Result(), true) {
					return false
				}
			case sil.EndAccess:
				if !scoped {
					return false
				}
				acc.ends = append(acc.ends, use)
			case sil.DeallocStack, sil.DebugValue:
				if scoped {
					return false
				}
				acc.ends = append(acc.ends, use)
			default:
				return false
			}
			acc.of[use] = true
		}
		return true
	}
	if !visit(addr, false) {
		return acc, false
	}
	return acc, true
}

func onlyAttr(in *sil.Inst, attr string) bool {
	a := in.Aux().Attrs
	return len(a) == 1 && a[0] == attr
}

// domInfo is the function's CFG with its dominator tree and dominance
// frontiers, over the blocks reachable from the entry.
type domInfo struct {
	blocks []*sil.Block
	succs  map[*sil.Block][]*sil.Block
	preds  map[*sil.Block][]*sil.Block
	rpo    map[*sil.Block]int // position in reverse postorder; absent = unreachable
	idom   map[*sil.Block]*sil.Block
	kids   map[*sil.Block][]*sil.Block
	df     map[*sil.Block][]*sil.Block
}

func newDomInfo(f *sil.Func) *domInfo {
	g := &domInfo{
		blocks: f.Blocks(),
		succs:  map[*sil.Block][]*sil.Block{},
		preds:  map[*sil.Block][]*sil.Block{},
		rpo:    map[*sil.Block]int{},
		idom:   map[*sil.Block]*sil.Block{},
		kids:   map[*sil.Block][]*sil.Block{},
		df:     map[*sil.Block][]*sil.Block{},
	}
	for _, b := range g.blocks {
		if t := b.Term(); t != nil {
			for _, s := range t.Successors() {
				g.succs[b] = append(g.succs[b], s)
				g.preds[s] = append(g.preds[s], b)
			}
		}
	}
	// Reverse postorder from the entry.
	var order []*sil.Block
	seen := map[*sil.Block]bool{}
	var dfs func(b *sil.Block)
	dfs = func(b *sil.Block) {
		seen[b] = true
		for _, s := range g.succs[b] {
			if !seen[s] {
				dfs(s)
			}
		}
		order = append(order, b)
	}
	entry := g.blocks[0]
	dfs(entry)
	for i, j := 0, len(order)-1; i < j; i, j = i+1, j-1 {
		order[i], order[j] = order[j], order[i]
	}
	for i, b := range order {
		g.rpo[b] = i
	}
	// Cooper, Harvey and Kennedy's iterative dominators.
	g.idom[entry] = entry
	intersect := func(a, b *sil.Block) *sil.Block {
		for a != b {
			for g.rpo[a] > g.rpo[b] {
				a = g.idom[a]
			}
			for g.rpo[b] > g.rpo[a] {
				b = g.idom[b]
			}
		}
		return a
	}
	for changed := true; changed; {
		changed = false
		for _, b := range order[1:] {
			var nd *sil.Block
			for _, p := range g.preds[b] {
				if _, ok := g.idom[p]; !ok {
					continue
				}
				if nd == nil {
					nd = p
				} else {
					nd = intersect(p, nd)
				}
			}
			if nd != nil && g.idom[b] != nd {
				g.idom[b] = nd
				changed = true
			}
		}
	}
	for _, b := range order[1:] {
		g.kids[g.idom[b]] = append(g.kids[g.idom[b]], b)
	}
	// Dominance frontiers, over reachable predecessors only.
	for _, b := range order {
		var reach []*sil.Block
		for _, p := range g.preds[b] {
			if g.reachable(p) {
				reach = append(reach, p)
			}
		}
		if len(reach) < 2 {
			continue
		}
		for _, p := range reach {
			for r := p; r != g.idom[b]; r = g.idom[r] {
				g.df[r] = appendOnce(g.df[r], b)
				if r == entry {
					break
				}
			}
		}
	}
	return g
}

func (g *domInfo) reachable(b *sil.Block) bool {
	_, ok := g.rpo[b]
	return ok
}

func appendOnce(bs []*sil.Block, b *sil.Block) []*sil.Block {
	for _, x := range bs {
		if x == b {
			return bs
		}
	}
	return append(bs, b)
}

// slotPlan is what promoting one slot does, worked out before anything is
// changed so that a slot that cannot be promoted is left as it was.
type slotPlan struct {
	phis  []*sil.Block          // blocks that take the value as an argument
	loads map[*sil.Inst]slotVal // what each load reads
	edges []slotEdge            // the value each branch into a phi block passes
}

// slotVal is the slot's value at some point: a value already in the
// function, or the argument a phi block is about to be given.
type slotVal struct {
	v   *sil.Value
	phi *sil.Block
}

type slotEdge struct {
	term *sil.Inst
	dest *sil.Block
	val  slotVal
}

func (g *domInfo) planSlot(alloc *sil.Inst, acc accesses) (slotPlan, bool) {
	plan := slotPlan{loads: map[*sil.Inst]slotVal{}}
	for in := range acc.of {
		if !g.reachable(in.Block()) {
			return plan, false
		}
	}

	// Liveness: a block is live-in when a load in it comes before any
	// store to the slot or its allocation, or when it passes the slot
	// through to a live-in successor untouched.
	type summary struct{ upLoad, kills bool }
	sum := map[*sil.Block]*summary{}
	mark := func(b *sil.Block) *summary {
		if s, ok := sum[b]; ok {
			return s
		}
		s := &summary{}
		sum[b] = s
		for _, in := range b.Insts() {
			switch {
			case in == alloc:
				s.kills = true
			case !acc.of[in]:
			case in.Op() == sil.Load:
				if !s.kills {
					s.upLoad = true
				}
			case in.Op() == sil.Store:
				s.kills = true
			}
			if s.kills {
				break
			}
		}
		return s
	}
	liveIn := map[*sil.Block]bool{}
	var work []*sil.Block
	for _, ld := range acc.loads {
		b := ld.Block()
		if mark(b).upLoad && !liveIn[b] {
			liveIn[b] = true
			work = append(work, b)
		}
	}
	for len(work) > 0 {
		b := work[len(work)-1]
		work = work[:len(work)-1]
		for _, p := range g.preds[b] {
			if !g.reachable(p) || liveIn[p] || mark(p).kills {
				continue
			}
			liveIn[p] = true
			work = append(work, p)
		}
	}

	// Phi blocks: the iterated dominance frontier of the stores, where the
	// slot is live.
	isPhi := map[*sil.Block]bool{}
	var defs []*sil.Block
	queued := map[*sil.Block]bool{}
	for _, st := range acc.stores {
		if b := st.Block(); !queued[b] {
			queued[b] = true
			defs = append(defs, b)
		}
	}
	for len(defs) > 0 {
		b := defs[len(defs)-1]
		defs = defs[:len(defs)-1]
		for _, d := range g.df[b] {
			if isPhi[d] || !liveIn[d] {
				continue
			}
			isPhi[d] = true
			plan.phis = append(plan.phis, d)
			if !queued[d] {
				queued[d] = true
				defs = append(defs, d)
			}
		}
	}
	for _, d := range plan.phis {
		// Every way into a phi block has to be able to pass the value.
		for _, p := range g.preds[d] {
			if !g.reachable(p) {
				return plan, false
			}
			switch p.Term().Op() {
			case sil.Br, sil.CondBr:
			default:
				return plan, false
			}
		}
		if d.IsEntry() {
			return plan, false
		}
	}

	// Renaming, down the dominator tree. A nil value is "nothing stored
	// yet", and reading it abandons the promotion.
	resolve := func(v *sil.Value) slotVal {
		if v.Inst() != nil {
			if r, ok := plan.loads[v.Inst()]; ok {
				return r
			}
		}
		return slotVal{v: v}
	}
	ok := true
	var walk func(b *sil.Block, cur *slotVal)
	walk = func(b *sil.Block, cur *slotVal) {
		if !ok {
			return
		}
		if isPhi[b] {
			cur = &slotVal{phi: b}
		}
		for _, in := range b.Insts() {
			switch {
			case in == alloc:
				cur = nil
			case !acc.of[in]:
			case in.Op() == sil.Load:
				if cur == nil {
					ok = false
					return
				}
				plan.loads[in] = *cur
			case in.Op() == sil.Store:
				v := resolve(in.Args()[0])
				cur = &v
			}
		}
		if t := b.Term(); t != nil {
			for _, s := range t.Successors() {
				if !isPhi[s] {
					continue
				}
				if cur == nil {
					ok = false
					return
				}
				plan.edges = append(plan.edges, slotEdge{term: t, dest: s, val: *cur})
			}
		}
		for _, k := range g.kids[b] {
			walk(k, cur)
		}
	}
	walk(g.blocks[0], nil)
	return plan, ok
}

// applySlot carries out a plan: the phi blocks take an argument, the
// branches into them pass it, the loads are replaced by what they read,
// and the slot and every access to it are erased.
func applySlot(alloc *sil.Inst, acc accesses, plan slotPlan) {
	t := alloc.Result().Type().Object()
	args := map[*sil.Block]*sil.Value{}
	for _, b := range plan.phis {
		args[b] = b.Arg(t, sil.None)
	}
	value := func(s slotVal) *sil.Value {
		if s.phi != nil {
			return args[s.phi]
		}
		return s.v
	}
	// A CondBr to the same block both ways passes the argument on each
	// edge; the plan has one entry per edge, and appendEdgeArg adds to
	// every arm that goes to dest, so one entry per (term, dest) is used.
	type key struct {
		term *sil.Inst
		dest *sil.Block
	}
	done := map[key]bool{}
	for _, e := range plan.edges {
		k := key{e.term, e.dest}
		if done[k] {
			continue
		}
		done[k] = true
		appendEdgeArg(e.term, e.dest, value(e.val))
	}
	for ld, s := range plan.loads {
		sil.ReplaceAllUses(ld.Result(), value(s))
	}
	erase := func(in *sil.Inst) {
		if b := in.Block(); b != nil {
			b.Erase(in)
		}
	}
	for _, in := range acc.ends {
		erase(in)
	}
	for _, in := range acc.loads {
		erase(in)
	}
	for _, in := range acc.stores {
		erase(in)
	}
	for _, in := range acc.scopes {
		erase(in)
	}
	erase(alloc)
}

// appendEdgeArg adds v to the arguments term passes to dest.
func appendEdgeArg(term *sil.Inst, dest *sil.Block, v *sil.Value) {
	aux := term.Aux()
	switch term.Op() {
	case sil.Br:
		a := append(append([]*sil.Value(nil), aux.Args...), v)
		aux.Args = a
		term.Rewrite(sil.Br, aux, a...)
	case sil.CondBr:
		cond := term.Args()[0]
		if aux.Dest == dest {
			aux.Args = append(append([]*sil.Value(nil), aux.Args...), v)
		}
		if aux.Else == dest {
			aux.ElseArgs = append(append([]*sil.Value(nil), aux.ElseArgs...), v)
		}
		ops := append([]*sil.Value{cond}, aux.Args...)
		ops = append(ops, aux.ElseArgs...)
		term.Rewrite(sil.CondBr, aux, ops...)
	}
}
