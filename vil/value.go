package vil

// An Ownership is what a value's holder owes it. Every SSA value in
// an OSSA function has one, and the two rules the verifier enforces
// are stated in terms of these.
type Ownership uint8

const (
	// None: the value owns nothing. A trivial type — an Int, a
	// metatype — is always None, and needs no copy and no destroy.
	None Ownership = iota
	// Owned: this value must be consumed exactly once on every path
	// out of its definition.
	Owned
	// Guaranteed: borrowed. It is valid only inside the scope that
	// produced it, and must not be consumed.
	Guaranteed
	// Unowned: neither owned nor guaranteed to outlive its use. It
	// must be copied before it is held.
	Unowned
)

func (o Ownership) String() string {
	switch o {
	case Owned:
		return "@owned"
	case Guaranteed:
		return "@guaranteed"
	case Unowned:
		return "@unowned"
	}
	return ""
}

// A Value is one SSA definition: a block argument, or one result of
// one instruction. Assigned once, used anywhere it dominates.
type Value struct {
	id    int
	typ   Type
	own   Ownership
	fn    *Func
	arg   *Block // non-nil for a block argument
	inst  *Inst  // non-nil for an instruction result
	index int    // which argument, or which result

	uses []*Inst
}

func (v *Value) Type() Type           { return v.typ }
func (v *Value) Ownership() Ownership { return v.own }
func (v *Value) Func() *Func          { return v.fn }

// ID is the value's dense number within its function, which is the
// %n the printer writes.
func (v *Value) ID() int { return v.id }

// Inst is the instruction that defined this value, or nil for a block
// argument.
func (v *Value) Inst() *Inst { return v.inst }

// Arg is the block this value is an argument of, or nil.
func (v *Value) Arg() *Block { return v.arg }

// Index is which argument of the block, or which result of the
// instruction, this value is.
func (v *Value) Index() int { return v.index }

// Uses are the instructions that take this value as an operand. Kept
// as the module is built, because every ownership question is a
// question about a value's uses.
func (v *Value) Uses() []*Inst { return v.uses }

// Block is where the value is defined.
func (v *Value) Block() *Block {
	if v.arg != nil {
		return v.arg
	}
	if v.inst != nil {
		return v.inst.blk
	}
	return nil
}

// Consumers are the uses that take ownership of this value. For an
// owned value there must be exactly one on every path, which is rule
// one and the reason this list is worth having.
func (v *Value) Consumers() []*Inst {
	var out []*Inst
	for _, in := range v.uses {
		for i, op := range in.args {
			if op == v && in.op.Consumes(i) {
				out = append(out, in)
				break
			}
		}
	}
	return out
}

func (f *Func) newValue(t Type, own Ownership) *Value {
	if t.Trivial() {
		own = None
	}
	v := &Value{id: f.nextID, typ: t, own: own, fn: f}
	f.nextID++
	return v
}
