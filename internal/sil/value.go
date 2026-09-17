package sil

// Ownership describes the ownership kind of an SSA value in OSSA form.
type Ownership uint8

const (
	None       Ownership = iota // Trivial value; requires no copy/destroy
	Owned                       // Must be consumed exactly once per path
	Guaranteed                  // Borrowed; valid only within borrow scope
	Unowned                     // Neither owned nor guaranteed; must copy before keeping
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

// A Value is an SSA definition: a block argument or an instruction result.
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

// ID returns the dense SSA index within its function (%n).
func (v *Value) ID() int { return v.id }

// Inst returns the defining instruction, or nil for a block argument.
func (v *Value) Inst() *Inst { return v.inst }

// Arg returns the defining block if this value is a block argument, or nil.
func (v *Value) Arg() *Block { return v.arg }

// Index returns the argument index or result index of this value.
func (v *Value) Index() int { return v.index }

// Uses returns all instructions that read this value.
func (v *Value) Uses() []*Inst { return v.uses }

// Block returns the block where the value is defined.
func (v *Value) Block() *Block {
	if v.arg != nil {
		return v.arg
	}
	if v.inst != nil {
		return v.inst.blk
	}
	return nil
}

// Consumers returns instructions that take ownership of this value.
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
