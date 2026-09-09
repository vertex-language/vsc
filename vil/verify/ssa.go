package verify

import (
	"fmt"

	"github.com/vertex-language/vsc/vil"
)

// The structural rules: what any SSA IR must hold, checked before the
// ownership rules because those are stated over a graph and a
// malformed graph makes them meaningless.

// structure checks one function's blocks, terminators, reachability
// and branch arities.
func (c *collector) structure(f *vil.Func, d *domTree) {
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
		}
	}
}

// signature checks the two places a body has to agree with the type
// it was declared with: the entry block's arguments are the
// parameters, and a return gives back the result.
func (c *collector) signature(f *vil.Func) {
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
		if t == nil || t.Op() != vil.Return {
			continue
		}
		if len(results) != 1 {
			continue // a multi-result or void signature: nothing to compare
		}
		if args := t.Args(); len(args) == 1 && !args[0].Type().Equal(results[0].Type) {
			c.at(b, indexOf(t), vil.Return, args[0], ErrSignature,
				fmt.Sprintf("returns %s, the result is %s",
					args[0].Type(), results[0].Type))
		}
	}
}

// terminator checks that a block ends in exactly one, at the end.
func (c *collector) terminator(b *vil.Block) {
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

// condition checks that a branch tests a machine bit. `cond_br` takes
// a Builtin.Int1, not a Bool: a Bool is a struct around one, and
// reaching through it is the caller's job.
func (c *collector) condition(b *vil.Block, i int, in *vil.Inst) {
	if in.Op() != vil.CondBr || len(in.Args()) == 0 {
		return
	}
	if t := in.Args()[0].Type(); !t.Equal(vil.Object(vil.BuiltinInt1)) {
		c.at(b, i, in.Op(), in.Args()[0], ErrSignature,
			"branches on "+t.String()+", which is not $Builtin.Int1")
	}
}

// branchTargets checks where a terminator goes and what it passes.
func (c *collector) branchTargets(b *vil.Block, i int, in *vil.Inst, f *vil.Func) {
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
	// switch_enum passes each case's payload as the destination's
	// argument, and the destination declares it; the arity that has
	// to hold is that a case with a payload goes to a block that
	// takes one.
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
func (c *collector) passes(b *vil.Block, i int, in *vil.Inst, dest *vil.Block, args []*vil.Value) {
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

// dominance checks that every use is reached by its definition: the
// rule that makes SSA mean anything.
func (c *collector) dominance(f *vil.Func, d *domTree) {
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
			// A branch's arguments are used at the branch, and are
			// held in Aux rather than in the operand list for the
			// edges that have their own.
			for _, v := range append(append([]*vil.Value{}, in.Aux().Args...), in.Aux().ElseArgs...) {
				c.reaches(v, b, i, in, d)
			}
		}
	}
}

// reaches checks one use against one definition.
func (c *collector) reaches(v *vil.Value, use *vil.Block, i int, in *vil.Inst, d *domTree) {
	if v == nil {
		return
	}
	def, at := definedIn(v)
	if def == nil {
		return
	}
	if def == use {
		// Within one block, the definition must come first. A block
		// argument is defined before every instruction, which is
		// what its index of -1 says.
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
