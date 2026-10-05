package sil

// Inst represents an instruction in a basic block.
type Inst struct {
	op      Op
	blk     *Block
	args    []*Value
	results []*Value
	aux     Aux
}

// Aux holds non-operand instruction metadata, constants, and branch targets.
type Aux struct {
	Name           string   // Symbol name (callee, global, or debug variable).
	Member         string   // Declaration reference (#Type.member).
	Int            int64    // Integer literal value or tuple index.
	Text           string   // String literal contents.
	Type           Type     // Secondary type parameter.
	Dest, Else     *Block   // Successor branch blocks.
	Args, ElseArgs []*Value // Successor branch arguments.
	Cases          []Case   // Switch cases.
	Attrs          []string // Bracketed modifiers (e.g. [copy], [init]).
}

// Case represents one branch target in a switch instruction.
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
