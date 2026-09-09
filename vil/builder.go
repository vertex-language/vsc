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

// AllocStackFor is a slot a source-level binding lives in, which SIL
// writes with the binding's name and kind beside the type:
//
//	%1 = alloc_stack $any Shape, let, name "v"
//
// A binding is storage rather than a value when its type is one that
// cannot be held in registers, and then this is what holds it.
func (b *Block) AllocStackFor(t Type, name, kind string) *Value {
	return b.add(AllocStack, Aux{Type: t, Name: name, Attrs: []string{kind}}, nil,
		t.Address()).Result()
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

// WitnessMethodOn looks a requirement up in the conformance a value
// carries.
//
// SIL names the opened archetype and finds the table through it; this
// names the existential itself, because that is where the table
// pointer is and this compiler has no archetypes to open.
func (b *Block) WitnessMethodOn(recv *Value, member string, t Type) *Value {
	return b.add(WitnessMethod, Aux{Member: member}, []*Value{recv}, t).Result()
}

// InitExistentialAddr is where the concrete value goes inside an
// existential: the address of the buffer, given the type that will be
// stored there.
// InitExistentialAddr opens an existential's storage for a value of a
// concrete type: the metadata that says what is inside goes in, the
// table for its conformance goes in beside it, and what comes back is
// where the value itself goes.
//
// The metadata is an operand rather than something looked up: which
// record it is depends on where the type was declared, and that is a
// question about names, which is this side's.
func (b *Block) InitExistentialAddr(addr *Value, concrete Type, meta *Value) *Value {
	args := []*Value{addr}
	if meta != nil {
		args = append(args, meta)
	}
	return b.add(InitExistentialAddr, Aux{Type: concrete},
		args, concrete.Address()).Result()
}

// OpenExistentialAddr is the address of the value inside an
// existential, for a call that will pass it to a witness.
func (b *Block) OpenExistentialAddr(addr *Value, t Type) *Value {
	return b.add(OpenExistentialAddr, Aux{}, []*Value{addr}, t).Result()
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

// ClassMetatype is a class's metadata, which is a value at run time
// rather than nothing.
//
// A struct's metatype carries no bits: the type is known statically
// and there is nothing left to say. A class's is a real pointer --
// its allocating initializer takes it as self, and what allocates the
// instance reads the size out of it -- and it comes from a function
// the module that declared the class exports. accessor names that
// function; the type says nothing about where it is.
func (b *Block) ClassMetatype(instance Type, accessor string) *Value {
	t := ThickMetatype(instance.Formal())
	return b.add(Metatype, Aux{Type: t, Name: accessor}, nil, t).Result()
}

// PointerToAddress takes a Builtin.RawPointer as the address of a
// value of a type. It moves no bits: what changes is what the type
// system will let the pointer be used for.
func (b *Block) PointerToAddress(p *Value, t Type) *Value {
	return b.add(PointerToAddress, Aux{Type: t}, []*Value{p}, t).Result()
}

// IndexAddr is the address of one element of an array of them: the
// base, and how many strides along it lies.
func (b *Block) IndexAddr(base, index *Value) *Value {
	return b.add(IndexAddr, Aux{}, []*Value{base, index}, base.Type()).Result()
}

// TypeMetadata is a type's metadata as a pointer.
//
// ClassMetatype's answer with its type forgotten, which is what a
// generic function's substitution argument is: swiftc's own call to
// _allocateUninitializedArray puts the count in x0 and Int32's
// metadata in x1, and the register holds a pointer whatever the type
// turns out to be. Spelling it as the metatype of one particular type
// would make one shared declaration of a generic function belong to
// whichever element type reached it first.
func (b *Block) TypeMetadata(instance Type, accessor string) *Value {
	return b.add(Metatype, Aux{Type: ThickMetatype(instance.Formal()), Name: accessor},
		nil, Object(BuiltinRawPointer)).Result()
}

// MetadataGlobal is a type's metadata read out of a record another
// library exports, at an offset inside it.
//
// `Any` is the one that needs it: libswiftCore exports the full
// record rather than an accessor, and the metadata proper begins past
// the value witness table pointer that starts every such record.
func (b *Block) MetadataGlobal(instance Type, symbol string, off int64) *Value {
	return b.add(Metatype, Aux{
		Type:  ThickMetatype(instance.Formal()),
		Name:  symbol,
		Int:   off,
		Attrs: []string{"global"},
	}, nil, Object(BuiltinRawPointer)).Result()
}

// ---- literals ----

// IntegerLiteral is a literal of a builtin integer type.
func (b *Block) IntegerLiteral(t Type, n int64) *Value {
	return b.add(IntegerLiteral, Aux{Type: t, Int: n}, nil, t).Result()
}

// FloatLiteral is a literal of a builtin floating-point type.
//
// Its bits rather than its value, which is how SIL writes one:
// `float_literal $Builtin.FPIEEE64, 0x3FF8000000000000`. A decimal
// spelling would be a second rounding between the source and the
// machine, and there is already one.
func (b *Block) FloatLiteral(t Type, bits int64) *Value {
	return b.add(FloatLiteral, Aux{Type: t, Int: bits}, nil, t).Result()
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

// TryApply calls a function that may fail, branching to one block
// where it did not and another where it did.
//
// It is the shape SILGen writes, and it is a terminator rather than
// an instruction with a result: what comes back is the normal block's
// argument on one path and nothing at all on the other. Swift carries
// the failure in a register beside the result, which is what makes
// one call have two edges out of it.
func (b *Block) TryApply(callee *Value, normal, failed *Block, args ...*Value) *Inst {
	return b.add(TryApply, Aux{Cases: []Case{
		{Member: "normal", Dest: normal},
		{Member: "error", Dest: failed},
	}}, append([]*Value{callee}, args...))
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
