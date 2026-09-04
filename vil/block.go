package vil

// A Block is a basic block: arguments, a run of instructions, and one
// terminator.
//
// Arguments rather than phi nodes, which is Swift's choice and VIR's:
// a value that differs by predecessor arrives as an argument of the
// block it is used in, and every branch to that block passes one.
// This is also what lets a block argument carry ownership, which a
// phi node could not.
type Block struct {
	fn    *Func
	index int
	args  []*Value
	insts []*Inst
}

func (b *Block) Func() *Func    { return b.fn }
func (b *Block) Index() int     { return b.index }
func (b *Block) Args() []*Value { return b.args }
func (b *Block) Insts() []*Inst { return b.insts }
func (b *Block) IsEntry() bool  { return b.index == 0 }

// Label is the block's name in the text form: bb0, bb1, …
func (b *Block) Label() string { return "bb" + itoa(b.index) }

// Term is the block's terminator, or nil where it has not been given
// one yet — which is a malformed block, and what the verifier says.
func (b *Block) Term() *Inst {
	if n := len(b.insts); n > 0 && b.insts[n-1].op.IsTerminator() {
		return b.insts[n-1]
	}
	return nil
}

// Preds are the blocks that branch to this one, in the order they
// appear in the function. Computed rather than stored, as VIR does
// it: a predecessor list that is stored is a predecessor list that
// can be wrong.
func (b *Block) Preds() []*Block {
	var out []*Block
	for _, p := range b.fn.blocks {
		if p == b {
			continue
		}
		t := p.Term()
		if t == nil {
			continue
		}
		for _, s := range t.Successors() {
			if s == b {
				out = append(out, p)
				break
			}
		}
	}
	return out
}

// Arg adds a block argument of the given type and ownership. An entry
// block's arguments are the function's parameters, and take their
// ownership from the calling convention.
func (b *Block) Arg(t Type, own Ownership) *Value {
	v := b.fn.newValue(t, own)
	v.arg = b
	v.index = len(b.args)
	b.args = append(b.args, v)
	return v
}

// add appends an instruction, records its operands' uses, and creates
// its results.
func (b *Block) add(op Op, aux Aux, args []*Value, results ...Type) *Inst {
	in := &Inst{op: op, blk: b, args: args, aux: aux}
	for _, a := range args {
		if a != nil {
			a.uses = append(a.uses, in)
		}
	}
	for i, t := range results {
		v := b.fn.newValue(t, resultOwnership(op, aux, t, args))
		v.inst = in
		v.index = i
		in.results = append(in.results, v)
	}
	b.insts = append(b.insts, in)
	return in
}

// resultOwnership is what an instruction's result owns. Most
// instructions either produce something owned or forward what they
// were given; the exceptions are named here because getting them
// wrong is what makes rule one unenforceable.
func resultOwnership(op Op, aux Aux, t Type, args []*Value) Ownership {
	if t.Trivial() {
		return None
	}
	// `load [copy]` and `load [take]` produce an owned value;
	// `load [trivial]` produces nothing to own.
	if op == Load {
		if hasAttr(aux, "copy") || hasAttr(aux, "take") {
			return Owned
		}
		return None
	}
	switch op {
	case CopyValue, AllocRef, AllocBox, Apply, TryApply, PartialApply,
		AllocExistentialBox, MoveValue:
		return Owned
	case BeginBorrow, OpenExistentialRef:
		return Guaranteed
	// A projection out of an aggregate borrows what it came from: the
	// field is alive exactly as long as the value that holds it.
	case StructExtract, TupleExtract, UncheckedEnumData:
		if len(args) > 0 {
			return args[0].own
		}
		return Guaranteed
	// An address is not a value: nothing owns it, and what it points
	// at is owned by whoever allocated it.
	case AllocStack, ProjectBox, RefElementAddr, StructElementAddr,
		TupleElementAddr, BeginAccess, InitExistentialAddr,
		OpenExistentialAddr, InitEnumDataAddr, MarkUninitialized:
		return None
	case Struct, Tuple, Enum:
		// An aggregate owns what was put into it.
		for _, a := range args {
			if a != nil && a.own == Owned {
				return Owned
			}
		}
		return None
	}
	return Owned
}

// hasAttr reports whether an instruction was written with the given
// bracketed modifier.
func hasAttr(aux Aux, name string) bool {
	for _, a := range aux.Attrs {
		if a == name {
			return true
		}
	}
	return false
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
