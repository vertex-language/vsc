package vil

// Mutation, for the passes that transform a module.
//
// The builder writes a module as it is first made; this rewrites one
// that already exists. They are kept apart on purpose: everything in
// builder.go appends and can only produce a well-formed instruction,
// while everything here changes what is already there and can leave a
// module in a state the verifier has to catch. A pass is expected to
// run the verifier after itself.
//
// The use lists are what make this possible and what it must keep
// right: every value knows which instructions read it, so replacing a
// value everywhere is a walk of that list rather than a walk of the
// function.

// ReplaceAllUses points every use of old at new.
//
// This is what erasing an instruction leaves behind. A `begin_borrow`
// that is no longer meaningful still has readers, and they must read
// what it borrowed from instead.
func ReplaceAllUses(old, new *Value) {
	if old == nil || new == nil || old == new {
		return
	}
	for _, in := range old.uses {
		for i, a := range in.args {
			if a == old {
				in.args[i] = new
				new.uses = append(new.uses, in)
			}
		}
		// A branch's arguments are held twice: once as operands, and
		// once on the edge they travel along.
		replaceIn(in.aux.Args, old, new)
		replaceIn(in.aux.ElseArgs, old, new)
	}
	old.uses = nil
}

func replaceIn(vs []*Value, old, new *Value) {
	for i, v := range vs {
		if v == old {
			vs[i] = new
		}
	}
}

// Rewrite turns one instruction into another in place, keeping its
// position in the block. The results it had are dropped, so a caller
// that is replacing an instruction which produced a value must have
// pointed that value's uses somewhere else first.
func (in *Inst) Rewrite(op Op, aux Aux, args ...*Value) {
	for _, a := range in.args {
		a.dropUse(in)
	}
	in.op, in.aux, in.args, in.results = op, aux, args, nil
	for _, a := range args {
		if a != nil {
			a.uses = append(a.uses, in)
		}
	}
}

// Reshape changes what an instruction is while keeping the values it
// already produced.
//
// Rewrite is the other half of the pair, and the difference is who
// the result belongs to. Rewrite replaces an instruction whose
// results have already been pointed somewhere else, and drops them.
// This is for the pass that turns one allocation into another, where
// every reader is reading the same thing before and after — the
// address of a variable — and what changed is only where the storage
// came from. Repointing those uses and then handing them back would
// be the same edit done twice.
//
// Neither op may take operands the other does not: the args are left
// exactly as they were.
func (in *Inst) Reshape(op Op, aux Aux) {
	in.op, in.aux = op, aux
}

// SetType changes what a value is.
//
// It goes with Reshape, for the same pass and the same reason: a box
// that became a stack slot holds an address where it held a box, and
// the value every reader already has is the one that has to say so.
func (v *Value) SetType(t Type) { v.typ = t }

// Erase removes an instruction from its block. Its operands forget
// it; its results, if it had any, must already have been replaced.
func (b *Block) Erase(in *Inst) {
	for i, other := range b.insts {
		if other != in {
			continue
		}
		for _, a := range in.args {
			a.dropUse(in)
		}
		b.insts = append(b.insts[:i], b.insts[i+1:]...)
		in.blk = nil
		return
	}
}

// SetOwnership changes what a value owns. A pass that erases the
// ownership form uses it to say that nothing does.
func (v *Value) SetOwnership(o Ownership) { v.own = o }

// ClearAttr removes a function attribute — `[ossa]`, once the
// ownership form has been lowered away.
func (f *Func) ClearAttr(name string) {
	for i, a := range f.attrs {
		if a == name {
			f.attrs = append(f.attrs[:i], f.attrs[i+1:]...)
			return
		}
	}
}

// dropUse forgets one reader.
func (v *Value) dropUse(in *Inst) {
	for i, u := range v.uses {
		if u == in {
			v.uses = append(v.uses[:i], v.uses[i+1:]...)
			return
		}
	}
}

// InsertBefore builds an instruction and puts it immediately before
// at, in the same block.
//
// The mutation API had no way to turn one instruction into several,
// only into a different one -- which is enough for a pass that
// substitutes and not enough for one that expands. Definite
// initialization is the second kind: an `assign` to a slot that owns
// what it holds becomes a load, a store and a release.
//
// The instruction is built the way the builder builds one, so its
// results carry the ownership the op gives them; what differs is
// only where it lands.
func (b *Block) InsertBefore(at *Inst, op Op, aux Aux, args []*Value, results ...Type) *Inst {
	in := b.add(op, aux, args, results...)
	// add appended it; move it to just before at.
	b.insts = b.insts[:len(b.insts)-1]
	for i, other := range b.insts {
		if other == at {
			b.insts = append(b.insts[:i], append([]*Inst{in}, b.insts[i:]...)...)
			return in
		}
	}
	// at is not in this block, so the instruction stays at the end
	// rather than vanishing.
	b.insts = append(b.insts, in)
	return in
}

// InsertAfter is InsertBefore's other half.
func (b *Block) InsertAfter(at *Inst, op Op, aux Aux, args []*Value, results ...Type) *Inst {
	in := b.add(op, aux, args, results...)
	b.insts = b.insts[:len(b.insts)-1]
	for i, other := range b.insts {
		if other == at {
			b.insts = append(b.insts[:i+1], append([]*Inst{in}, b.insts[i+1:]...)...)
			return in
		}
	}
	b.insts = append(b.insts, in)
	return in
}
