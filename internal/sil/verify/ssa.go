package verify

import (
	"fmt"

	"github.com/vertex-language/vsc/internal/sil"
)

// Structural validity checks for SIL CFG and SSA form.

// structure verifies block terminators, reachability, signatures, and branch targets.
func (c *collector) structure(f *sil.Func, d *domTree) {
	if len(f.Blocks()) == 0 {
		c.fnErr(ErrNoEntry, "")
		return
	}
	c.signature(f)

	for _, b := range f.Blocks() {
		c.terminator(b)
		if !d.Reachable(b) {
			c.at(b, -1, "", nil, ErrUnreachable, "")
		}
		for i, in := range b.Insts() {
			c.branchTargets(b, i, in, f)
			c.condition(b, i, in)
			c.stored(b, i, in)
		}
	}
}

// signature verifies entry block arguments against params and return instruction types against results.
func (c *collector) signature(f *sil.Func) {
	entry := f.Entry()
	params := f.Type().Params
	args := entry.Args()
	if len(args) != len(params) {
		c.at(entry, -1, "", nil, ErrSignature,
			fmt.Sprintf("entry block takes %d arguments, the type has %d parameters",
				len(args), len(params)))
		return
	}
	for i, a := range args {
		if !a.Type().Equal(params[i].Type) {
			c.at(entry, -1, "", a, ErrSignature,
				fmt.Sprintf("argument %d is %s, the parameter is %s",
					i, a.Type(), params[i].Type))
		}
	}

	results := f.Type().Results
	for _, b := range f.Blocks() {
		t := b.Term()
		if t == nil || t.Op() != sil.Return {
			continue
		}
		if len(results) != 1 {
			continue
		}
		if args := t.Args(); len(args) == 1 && !args[0].Type().Equal(results[0].Type) {
			c.at(b, indexOf(t), sil.Return, args[0], ErrSignature,
				fmt.Sprintf("returns %s, the result is %s",
					args[0].Type(), results[0].Type))
		}
	}
}

// terminator verifies that a block ends with exactly one terminator.
func (c *collector) terminator(b *sil.Block) {
	insts := b.Insts()
	if len(insts) == 0 {
		c.at(b, -1, "", nil, ErrTerminator, "the block is empty")
		return
	}
	for i, in := range insts[:len(insts)-1] {
		if in.Op().IsTerminator() {
			c.at(b, i, in.Op(), nil, ErrTerminator,
				"a terminator is followed by another instruction")
		}
	}
	if last := insts[len(insts)-1]; !last.Op().IsTerminator() {
		c.at(b, len(insts)-1, last.Op(), nil, ErrTerminator, "")
	}
}

// condition checks that cond_br branches on Builtin.Int1.
func (c *collector) condition(b *sil.Block, i int, in *sil.Inst) {
	if in.Op() != sil.CondBr || len(in.Args()) == 0 {
		return
	}
	if t := in.Args()[0].Type(); !t.Equal(sil.Object(sil.BuiltinInt1)) {
		c.at(b, i, in.Op(), in.Args()[0], ErrSignature,
			"branches on "+t.String()+", which is not $Builtin.Int1")
	}
}

// stored checks that store or assign value types match destination address types.
func (c *collector) stored(b *sil.Block, i int, in *sil.Inst) {
	if in.Op() != sil.Store && in.Op() != sil.Assign {
		return
	}
	args := in.Args()
	if len(args) != 2 || args[0] == nil || args[1] == nil {
		return
	}
	value, addr := args[0].Type(), args[1].Type()
	if !addr.IsAddress() {
		c.at(b, i, in.Op(), args[1], ErrStoreType,
			"the destination is "+addr.String()+", which is not an address")
		return
	}
	if !value.Equal(addr.Object()) {
		c.at(b, i, in.Op(), args[0], ErrStoreType,
			"stores "+value.String()+" into an address of "+addr.Object().String())
	}
}

// branchTargets checks where a terminator goes and what it passes.
func (c *collector) branchTargets(b *sil.Block, i int, in *sil.Inst, f *sil.Func) {
	if !in.Op().IsTerminator() {
		return
	}
	entry := f.Entry()
	for _, s := range in.Successors() {
		if s == entry {
			c.at(b, i, in.Op(), nil, ErrEntryTarget, "")
		}
		if s.Func() != f {
			c.at(b, i, in.Op(), nil, ErrBranchArity,
				"the destination is in another function")
		}
	}

	aux := in.Aux()
	if aux.Dest != nil {
		c.passes(b, i, in, aux.Dest, aux.Args)
	}
	if aux.Else != nil {
		c.passes(b, i, in, aux.Else, aux.ElseArgs)
	}
	// Check case destination arities.
	for _, cs := range aux.Cases {
		if cs.Dest == nil {
			continue
		}
		if n := len(cs.Dest.Args()); n > 1 {
			c.at(b, i, in.Op(), nil, ErrBranchArity,
				fmt.Sprintf("case %s goes to a block taking %d arguments", cs.Member, n))
		}
	}
}

// passes checks one branch edge's arguments against the destination.
func (c *collector) passes(b *sil.Block, i int, in *sil.Inst, dest *sil.Block, args []*sil.Value) {
	params := dest.Args()
	if len(args) != len(params) {
		c.at(b, i, in.Op(), nil, ErrBranchArity,
			fmt.Sprintf("passes %d arguments to %s, which takes %d",
				len(args), dest.Label(), len(params)))
		return
	}
	for j, a := range args {
		if a == nil {
			continue
		}
		if !a.Type().Equal(params[j].Type()) {
			c.at(b, i, in.Op(), a, ErrBranchArity,
				fmt.Sprintf("argument %d is %s, %s takes %s",
					j, a.Type(), dest.Label(), params[j].Type()))
		}
	}
}

// dominance verifies that value definitions dominate all uses.
func (c *collector) dominance(f *sil.Func, d *domTree) {
	for _, b := range f.Blocks() {
		if !d.Reachable(b) {
			continue
		}
		for i, in := range b.Insts() {
			for _, v := range in.Args() {
				if v == nil {
					continue
				}
				c.reaches(v, b, i, in, d)
			}
			// Check edge arguments stored in Aux.
			for _, v := range append(append([]*sil.Value{}, in.Aux().Args...), in.Aux().ElseArgs...) {
				c.reaches(v, b, i, in, d)
			}
		}
	}
}

// reaches checks one use against one definition.
func (c *collector) reaches(v *sil.Value, use *sil.Block, i int, in *sil.Inst, d *domTree) {
	if v == nil {
		return
	}
	def, at := definedIn(v)
	if def == nil {
		return
	}
	if def == use {
		// Within a single block, definition must precede use (block args have index -1).
		if at >= i {
			c.at(use, i, in.Op(), v, ErrDominance,
				"the definition comes later in the same block")
		}
		return
	}
	if !d.Dominates(def, use) {
		c.at(use, i, in.Op(), v, ErrDominance,
			"defined in "+def.Label())
	}
}
