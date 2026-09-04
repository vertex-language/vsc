package vil

// An Inst is one instruction: an opcode, its operands, its results,
// and whatever else that opcode is written with.
//
// One struct rather than one type per opcode, as ir does it. What
// varies between opcodes beyond the operands is small and named in
// Aux, and an instruction that needs a new field adds one there
// rather than a new type to every switch in the package.
type Inst struct {
	op      Op
	blk     *Block
	args    []*Value
	results []*Value
	aux     Aux
}

// Aux is the part of an instruction that is not an operand: a name, a
// literal, a field, the blocks a terminator goes to. Each opcode uses
// the fields its text form writes and leaves the rest zero.
type Aux struct {
	// Name is a symbol: function_ref's callee, alloc_global's global,
	// debug_value's source name.
	Name string
	// Member is a declaration reference: struct_extract's field,
	// enum's case, class_method's method — printed as `#Type.name`.
	Member string
	// Int is an integer literal's value, or a tuple_extract's index.
	Int int64
	// Text is a string literal's contents.
	Text string
	// Type is a second type the opcode names: metatype's instance,
	// alloc_stack's element, an existential's concrete type.
	Type Type
	// Dest, Else are where a terminator goes.
	Dest, Else *Block
	// Args, ElseArgs are what it passes.
	Args, ElseArgs []*Value
	// Cases are switch_enum's arms, in written order.
	Cases []Case
	// Attrs are the bracketed modifiers: [take], [init], [var_decl],
	// [lexical], [callee_guaranteed].
	Attrs []string
}

// A Case is one arm of a switch: the enum element it matches and the
// block it goes to. A default arm has an empty Member.
type Case struct {
	Member string
	Dest   *Block
}

func (in *Inst) Op() Op            { return in.op }
func (in *Inst) Block() *Block     { return in.blk }
func (in *Inst) Args() []*Value    { return in.args }
func (in *Inst) Results() []*Value { return in.results }
func (in *Inst) Aux() Aux          { return in.aux }

// Result is the single result of an instruction that has one, or nil.
func (in *Inst) Result() *Value {
	if len(in.results) == 1 {
		return in.results[0]
	}
	return nil
}

// Func is the function the instruction belongs to.
func (in *Inst) Func() *Func {
	if in.blk == nil {
		return nil
	}
	return in.blk.fn
}

// Successors are the blocks control may reach from this instruction.
// Empty for everything but a terminator.
func (in *Inst) Successors() []*Block {
	var out []*Block
	if in.aux.Dest != nil {
		out = append(out, in.aux.Dest)
	}
	if in.aux.Else != nil {
		out = append(out, in.aux.Else)
	}
	for _, c := range in.aux.Cases {
		if c.Dest != nil {
			out = append(out, c.Dest)
		}
	}
	return out
}
