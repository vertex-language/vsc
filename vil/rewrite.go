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
