package vil

// The builder: one method per instruction, named as the instruction
// is named with the underscores removed. `copy_value` is CopyValue,
// `begin_borrow` is BeginBorrow. A reader with SIL open should never
// have to translate.
//
// Instructions are appended to the block they are called on, and a
// block is finished when it is given a terminator.

// ---- ownership ----

// CopyValue produces an owned copy of a value the caller keeps.
func (b *Block) CopyValue(v *Value) *Value {
	return b.add(CopyValue, Aux{}, []*Value{v}, v.typ).Result()
}

// DestroyValue consumes an owned value.
func (b *Block) DestroyValue(v *Value) *Inst {
	return b.add(DestroyValue, Aux{}, []*Value{v})
}

// BeginBorrow opens a borrow scope over an owned value.
func (b *Block) BeginBorrow(v *Value, attrs ...string) *Value {
	return b.add(BeginBorrow, Aux{Attrs: attrs}, []*Value{v}, v.typ).Result()
}

// EndBorrow closes one.
func (b *Block) EndBorrow(v *Value) *Inst {
	return b.add(EndBorrow, Aux{}, []*Value{v})
}

// MoveValue transfers ownership, ending the source's lifetime.
func (b *Block) MoveValue(v *Value, attrs ...string) *Value {
	return b.add(MoveValue, Aux{Attrs: attrs}, []*Value{v}, v.typ).Result()
}

// ExtendLifetime keeps a value alive to this point without using it,
// which is what a `defer` and an unused binding need.
func (b *Block) ExtendLifetime(v *Value) *Inst {
	return b.add(ExtendLifetime, Aux{}, []*Value{v})
}

// ---- memory ----

// AllocStack allocates a local of type t and yields its address.
func (b *Block) AllocStack(t Type, attrs ...string) *Value {
	return b.add(AllocStack, Aux{Type: t, Attrs: attrs}, nil, t.Address()).Result()
}

// DeallocStack releases it, in reverse order of allocation.
func (b *Block) DeallocStack(addr *Value) *Inst {
	return b.add(DeallocStack, Aux{}, []*Value{addr})
}

// AllocRef allocates an instance of a class.
func (b *Block) AllocRef(t Type) *Value {
	return b.add(AllocRef, Aux{Type: t}, nil, t.Object()).Result()
}

// DeallocRef frees one.
func (b *Block) DeallocRef(v *Value) *Inst {
	return b.add(DeallocRef, Aux{}, []*Value{v})
}

// AllocBox allocates a heap box for a variable, which is what a
// `var` is in raw VIL: a box, borrowed for the variable's scope, and
// projected to get at what it holds.
func (b *Block) AllocBox(elem Type, name string, attrs ...string) *Value {
	boxed := Box(elem.Formal())
	return b.add(AllocBox, Aux{Type: boxed, Name: name, Attrs: attrs}, nil,
		boxed).Result()
}

// ProjectBox yields the address of what a box holds.
func (b *Block) ProjectBox(box *Value, field int, t Type) *Value {
	return b.add(ProjectBox, Aux{Int: int64(field)}, []*Value{box}, t.Address()).Result()
}

// Load reads a value from an address. The attribute says what happens
// to the ownership: copy, take, or trivial.
func (b *Block) Load(addr *Value, attr string) *Value {
	return b.add(Load, Aux{Attrs: []string{attr}}, []*Value{addr},
		addr.typ.Object()).Result()
}

// Store writes a value to an address. The attribute says whether the
// address held anything before: init, or assign.
func (b *Block) Store(v, addr *Value, attr string) *Inst {
	return b.add(Store, Aux{Attrs: []string{attr}}, []*Value{v, addr})
}

// Assign stores to an address that already holds a value, which is
// what an assignment to a var is before definite initialization has
// decided whether it was the first one.
func (b *Block) Assign(v, addr *Value) *Inst {
	return b.add(Assign, Aux{}, []*Value{v, addr})
}

// CopyAddr copies between addresses, which is how an address-only
// type moves.
func (b *Block) CopyAddr(src, dst *Value, attrs ...string) *Inst {
	return b.add(CopyAddr, Aux{Attrs: attrs}, []*Value{src, dst})
}

// DestroyAddr destroys what an address holds.
func (b *Block) DestroyAddr(addr *Value) *Inst {
	return b.add(DestroyAddr, Aux{}, []*Value{addr})
}

// BeginAccess opens an exclusive or shared access to memory, which is
// what the exclusivity rules are checked over.
func (b *Block) BeginAccess(addr *Value, attrs ...string) *Value {
	return b.add(BeginAccess, Aux{Attrs: attrs}, []*Value{addr}, addr.typ).Result()
}

// EndAccess closes one.
func (b *Block) EndAccess(v *Value) *Inst {
	return b.add(EndAccess, Aux{}, []*Value{v})
}

// MarkUninitialized marks memory that definite initialization must
// prove is written before it is read. The pass removes it.
func (b *Block) MarkUninitialized(addr *Value, kind string) *Value {
	return b.add(MarkUninitialized, Aux{Attrs: []string{kind}},
		[]*Value{addr}, addr.typ).Result()
}

// ---- aggregates ----

// Struct builds a struct value from its fields, in declaration order.
func (b *Block) Struct(t Type, fields ...*Value) *Value {
	return b.add(Struct, Aux{Type: t}, fields, t.Object()).Result()
}

// StructExtract reads one field of a struct value.
func (b *Block) StructExtract(v *Value, member string, t Type) *Value {
	return b.add(StructExtract, Aux{Member: member}, []*Value{v}, t.Object()).Result()
}

// StructElementAddr yields the address of one field.
func (b *Block) StructElementAddr(addr *Value, member string, t Type) *Value {
	return b.add(StructElementAddr, Aux{Member: member}, []*Value{addr},
		t.Address()).Result()
}

// Tuple builds a tuple.
func (b *Block) Tuple(t Type, elems ...*Value) *Value {
	return b.add(Tuple, Aux{Type: t}, elems, t.Object()).Result()
}

// TupleExtract reads one element by position.
func (b *Block) TupleExtract(v *Value, index int, t Type) *Value {
	return b.add(TupleExtract, Aux{Int: int64(index)}, []*Value{v}, t.Object()).Result()
}

// DestructureTuple takes a tuple apart into all of its elements at
// once, which is what a pattern match produces.
func (b *Block) DestructureTuple(v *Value, elems ...Type) []*Value {
	return b.add(DestructureTuple, Aux{}, []*Value{v}, elems...).results
}

// Enum builds an enum value: a case, and its payload where it has one.
func (b *Block) Enum(t Type, member string, payload *Value) *Value {
	var args []*Value
	if payload != nil {
		args = []*Value{payload}
	}
	return b.add(Enum, Aux{Type: t, Member: member}, args, t.Object()).Result()
}

// UncheckedEnumData reads a case's payload, having already matched it.
func (b *Block) UncheckedEnumData(v *Value, member string, t Type) *Value {
	return b.add(UncheckedEnumData, Aux{Member: member}, []*Value{v}, t.Object()).Result()
}

// ---- references and calls ----

// RefElementAddr yields the address of a stored property of a class
// instance.
func (b *Block) RefElementAddr(ref *Value, member string, t Type) *Value {
	return b.add(RefElementAddr, Aux{Member: member}, []*Value{ref},
		t.Address()).Result()
}

// FunctionRef names a function as a value.
func (b *Block) FunctionRef(fn *Func) *Value {
	t := Type{formal: fn.typ}
	return b.add(FunctionRef, Aux{Name: fn.name}, nil, t).Result()
}

// ClassMethod looks a method up in an instance's vtable.
func (b *Block) ClassMethod(ref *Value, member string, t Type) *Value {
	return b.add(ClassMethod, Aux{Member: member}, []*Value{ref}, t).Result()
}

// WitnessMethod looks a requirement up in a conformance.
func (b *Block) WitnessMethod(member string, t Type) *Value {
	return b.add(WitnessMethod, Aux{Member: member}, nil, t).Result()
}

// Apply calls a function value.
func (b *Block) Apply(callee *Value, result Type, args ...*Value) *Value {
	return b.add(Apply, Aux{}, append([]*Value{callee}, args...), result).Result()
}

// ThinToThickFunction gives a function with no context the shape of one
// that has a context. A closure that captures nothing is a top-level
// function and a value of function type is thick, so the two have to
// meet somewhere, and this is where SILGen puts it.
func (b *Block) ThinToThickFunction(fn *Value, t Type) *Value {
	return b.add(ThinToThickFunction, Aux{Type: t}, []*Value{fn}, t).Result()
}

// PartialApply binds some arguments and produces a thick function —
// which is what a closure is.
func (b *Block) PartialApply(callee *Value, t Type, args ...*Value) *Value {
	return b.add(PartialApply, Aux{Attrs: []string{"callee_guaranteed"}},
		append([]*Value{callee}, args...), t).Result()
}

// Metatype names a type as a value. The result is thin: the type is
// known here, so there is nothing to carry.
func (b *Block) Metatype(instance Type) *Value {
	t := ThinMetatype(instance.Formal())
	return b.add(Metatype, Aux{Type: t}, nil, t).Result()
}

// ---- literals ----

// IntegerLiteral is a literal of a builtin integer type.
func (b *Block) IntegerLiteral(t Type, n int64) *Value {
	return b.add(IntegerLiteral, Aux{Type: t, Int: n}, nil, t).Result()
}

// StringLiteral is a literal of Builtin.RawPointer, with an encoding.
func (b *Block) StringLiteral(s string, encoding string) *Value {
	return b.add(StringLiteral, Aux{Text: s, Attrs: []string{encoding}}, nil,
		Object(BuiltinRawPointer)).Result()
}

// ---- builtins ----

// Builtin calls a machine instruction by name. This is where an
// operator's declaration stops being a call and becomes an add: core
// says which instruction a `+` on Int is, and this emits it.
func (b *Block) Builtin(name string, result Type, args ...*Value) *Value {
	return b.add(BuiltinCall, Aux{Name: name}, args, result).Result()
}

// CondFail traps when its operand is true, which is how checked
// arithmetic reports an overflow.
func (b *Block) CondFail(cond *Value, message string) *Inst {
	return b.add(CondFail, Aux{Text: message}, []*Value{cond})
}

// ---- debug ----

// DebugValue records the source name of a value, which is what a
// diagnostic and a debugger need and what nothing else reads.
func (b *Block) DebugValue(v *Value, name string, attrs ...string) *Inst {
	return b.add(DebugValue, Aux{Name: name, Attrs: attrs}, []*Value{v})
}

// ---- terminators ----

// Br branches unconditionally, passing the destination's arguments.
func (b *Block) Br(dest *Block, args ...*Value) *Inst {
	return b.add(Br, Aux{Dest: dest, Args: args}, args)
}

// CondBr branches on a Builtin.Int1.
func (b *Block) CondBr(cond *Value, yes *Block, yesArgs []*Value, no *Block, noArgs []*Value) *Inst {
	operands := append([]*Value{cond}, yesArgs...)
	operands = append(operands, noArgs...)
	return b.add(CondBr, Aux{Dest: yes, Args: yesArgs, Else: no, ElseArgs: noArgs}, operands)
}

// SwitchEnum dispatches on which case an enum holds, passing the
// payload to the destination as a block argument.
func (b *Block) SwitchEnum(v *Value, cases ...Case) *Inst {
	return b.add(SwitchEnum, Aux{Cases: cases}, []*Value{v})
}

// Return gives a value back to the caller, consuming it.
func (b *Block) Return(v *Value) *Inst {
	return b.add(Return, Aux{}, []*Value{v})
}

// Throw returns along the error edge.
func (b *Block) Throw(v *Value) *Inst {
	return b.add(Throw, Aux{}, []*Value{v})
}

// Unreachable says control does not arrive here.
func (b *Block) Unreachable() *Inst {
	return b.add(Unreachable, Aux{}, nil)
}
