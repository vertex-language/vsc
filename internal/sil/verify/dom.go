package verify

import "github.com/vertex-language/vsc/internal/sil"

// Dominator tree computation using Cooper-Harvey-Kennedy iterative algorithm.

// A domTree is the immediate-dominator relation of one function.
type domTree struct {
	fn    *sil.Func
	order []*sil.Block // reverse postorder
	rpo   map[*sil.Block]int
	idom  map[*sil.Block]*sil.Block
}

// buildDom computes the dominator tree of f for all reachable blocks.
func buildDom(f *sil.Func) *domTree {
	d := &domTree{
		fn:   f,
		rpo:  map[*sil.Block]int{},
		idom: map[*sil.Block]*sil.Block{},
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
			var new *sil.Block
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

// intersect finds the nearest common dominator of blocks a and b.
func (d *domTree) intersect(a, b *sil.Block) *sil.Block {
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
func (d *domTree) Reachable(b *sil.Block) bool {
	_, ok := d.rpo[b]
	return ok
}

// Dominates reports whether block a dominates block b. A block dominates itself.
func (d *domTree) Dominates(a, b *sil.Block) bool {
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

// reversePostorder returns reachable blocks in reverse postorder.
func reversePostorder(f *sil.Func) []*sil.Block {
	var post []*sil.Block
	seen := map[*sil.Block]bool{}

	var walk func(b *sil.Block)
	walk = func(b *sil.Block) {
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

// definedIn returns the block and instruction index defining v (-1 for block arguments).
func definedIn(v *sil.Value) (*sil.Block, int) {
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

// indexOf returns the instruction index within its enclosing block.
func indexOf(in *sil.Inst) int {
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
