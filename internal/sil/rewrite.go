package sil

// In-place mutation helpers for SIL passes.

// ReplaceAllUses replaces every use of old with new.
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
		// Update edge arguments if present.
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

// Rewrite replaces an instruction in place and drops its results.
// Callers must replace existing result uses before calling Rewrite.
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

// Reshape changes the opcode and aux of an instruction while preserving its results and arguments.
func (in *Inst) Reshape(op Op, aux Aux) {
	in.op, in.aux = op, aux
}

// SetType sets the type of a value.
func (v *Value) SetType(t Type) { v.typ = t }

// Erase removes an instruction from its block and drops operand uses.
// Results must have been replaced prior to erasure.
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

// SetOwnership updates the ownership of a value.
func (v *Value) SetOwnership(o Ownership) { v.own = o }

// ClearAttr removes a named function attribute (e.g. "[ossa]").
func (f *Func) ClearAttr(name string) {
	for i, a := range f.attrs {
		if a == name {
			f.attrs = append(f.attrs[:i], f.attrs[i+1:]...)
			return
		}
	}
}

// dropUse removes an instruction from the value's use list.
func (v *Value) dropUse(in *Inst) {
	for i, u := range v.uses {
		if u == in {
			v.uses = append(v.uses[:i], v.uses[i+1:]...)
			return
		}
	}
}

// InsertBefore builds an instruction and inserts it immediately before at in the block.
func (b *Block) InsertBefore(at *Inst, op Op, aux Aux, args []*Value, results ...Type) *Inst {
	in := b.add(op, aux, args, results...)
	b.insts = b.insts[:len(b.insts)-1]
	for i, other := range b.insts {
		if other == at {
			b.insts = append(b.insts[:i], append([]*Inst{in}, b.insts[i:]...)...)
			return in
		}
	}
	// If at is not found in b, append to the end.
	b.insts = append(b.insts, in)
	return in
}

// InsertAfter builds an instruction and inserts it immediately after at in the block.
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
