package verify

import "github.com/vertex-language/vsc/vil"

// Dominance, and the reachability it is computed over.
//
// A block d dominates a block b when every path from the entry to b
// passes through d. It is what makes SSA checkable — a use must be
// dominated by its definition — and it is what the ownership rules
// are stated over, since "on every path" is a statement about the
// graph and not about the instruction list.
//
// Cooper, Harvey and Kennedy's iterative algorithm: simple, fast
// enough for function-sized graphs, and short enough to be read.

// A domTree is the immediate-dominator relation of one function.
type domTree struct {
	fn    *vil.Func
	order []*vil.Block // reverse postorder
	rpo   map[*vil.Block]int
	idom  map[*vil.Block]*vil.Block
}

// buildDom computes the dominator tree of f over the blocks reachable
// from its entry.
func buildDom(f *vil.Func) *domTree {
	d := &domTree{
		fn:   f,
		rpo:  map[*vil.Block]int{},
		idom: map[*vil.Block]*vil.Block{},
	}
	if len(f.Blocks()) == 0 {
		return d
	}
	d.order = reversePostorder(f)
	for i, b := range d.order {
		d.rpo[b] = i
	}

	entry := d.order[0]
	d.idom[entry] = entry
	for changed := true; changed; {
		changed = false
		for _, b := range d.order[1:] {
			var new *vil.Block
			for _, p := range b.Preds() {
				if _, seen := d.rpo[p]; !seen {
					continue // unreachable: it dominates nothing
				}
				if d.idom[p] == nil {
					continue // not yet processed on this round
				}
				if new == nil {
					new = p
					continue
				}
				new = d.intersect(p, new)
			}
			if new != nil && d.idom[b] != new {
				d.idom[b] = new
				changed = true
			}
		}
	}
	return d
}

// intersect walks two blocks up the tree until they meet, which is
// their nearest common dominator.
func (d *domTree) intersect(a, b *vil.Block) *vil.Block {
	for a != b {
		for d.rpo[a] > d.rpo[b] {
			a = d.idom[a]
		}
		for d.rpo[b] > d.rpo[a] {
			b = d.idom[b]
		}
	}
	return a
}

// Reachable reports whether any path from the entry reaches b.
func (d *domTree) Reachable(b *vil.Block) bool {
	_, ok := d.rpo[b]
	return ok
}

// Dominates reports whether every path from the entry to b passes
// through a. A block dominates itself.
func (d *domTree) Dominates(a, b *vil.Block) bool {
	if a == b {
		return true
	}
	if !d.Reachable(a) || !d.Reachable(b) {
		return false
	}
	for cur := b; ; {
		next := d.idom[cur]
		if next == nil || next == cur {
			return false
		}
		if next == a {
			return true
		}
		cur = next
	}
}

// reversePostorder is the blocks reachable from the entry, ordered so
// that every block appears before the blocks it dominates.
func reversePostorder(f *vil.Func) []*vil.Block {
	var post []*vil.Block
	seen := map[*vil.Block]bool{}

	var walk func(b *vil.Block)
	walk = func(b *vil.Block) {
		if b == nil || seen[b] {
			return
		}
		seen[b] = true
		if t := b.Term(); t != nil {
			for _, s := range t.Successors() {
				walk(s)
			}
		}
		post = append(post, b)
	}
	walk(f.Entry())

	for i, j := 0, len(post)-1; i < j; i, j = i+1, j-1 {
		post[i], post[j] = post[j], post[i]
	}
	return post
}

// definedIn is the block a value is defined in, and the index of the
// instruction that defined it — which is what "before" means inside
// one block.
func definedIn(v *vil.Value) (*vil.Block, int) {
	if b := v.Arg(); b != nil {
		return b, -1 // a block argument is defined before every instruction
	}
	in := v.Inst()
	if in == nil {
		return nil, -1
	}
	b := in.Block()
	for i, other := range b.Insts() {
		if other == in {
			return b, i
		}
	}
	return b, -1
}

// indexOf is where an instruction sits in its block.
func indexOf(in *vil.Inst) int {
	if in == nil || in.Block() == nil {
		return -1
	}
	for i, other := range in.Block().Insts() {
		if other == in {
			return i
		}
	}
	return -1
}
