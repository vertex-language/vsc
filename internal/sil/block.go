package sil

// Block is a basic block containing parameters, a sequence of instructions, and a terminator.
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

// Term returns the block's terminator instruction, or nil if none.
func (b *Block) Term() *Inst {
	if n := len(b.insts); n > 0 && b.insts[n-1].op.IsTerminator() {
		return b.insts[n-1]
	}
	return nil
}

// Preds returns predecessor blocks that branch to this block.
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

// Arg appends a block argument with type and ownership.
func (b *Block) Arg(t Type, own Ownership) *Value {
	v := b.fn.newValue(t, own)
	v.arg = b
	v.index = len(b.args)
	b.args = append(b.args, v)
	return v
}

// add appends an instruction and creates its result values.
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

// resultOwnership determines result value ownership based on opcode and operand types.
func resultOwnership(op Op, aux Aux, t Type, args []*Value) Ownership {
	if t.Trivial() {
		return None
	}
	// `load [copy]` and `load [take]` produce an owned value; `load [trivial]` produces None.
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
	case StructExtract, TupleExtract, UncheckedEnumData, Upcast:
		if len(args) > 0 {
			return args[0].own
		}
		return Guaranteed
	case FunctionRef, WitnessMethod, ClassMethod, Metatype:
		return None
	case AllocStack, ProjectBox, RefElementAddr, StructElementAddr,
		TupleElementAddr, BeginAccess, InitExistentialAddr,
		OpenExistentialAddr, InitEnumDataAddr, UncheckedTakeEnumDataAddr,
		PointerToAddress, IndexAddr, GlobalAddr:
		return None
	case AddressToPointer:
		return None
	// MarkUninitialized inherits ownership from its storage operand.
	case MarkUninitialized:
		if len(args) > 0 && args[0] != nil {
			return args[0].own
		}
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

// hasAttr reports whether aux contains the named bracketed modifier.
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
