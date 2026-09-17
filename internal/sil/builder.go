package sil

// Instruction builder methods for Block.

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

// EndBorrow closes a borrow scope.
func (b *Block) EndBorrow(v *Value) *Inst {
	return b.add(EndBorrow, Aux{}, []*Value{v})
}

// MoveValue transfers ownership, ending the source's lifetime.
func (b *Block) MoveValue(v *Value, attrs ...string) *Value {
	return b.add(MoveValue, Aux{Attrs: attrs}, []*Value{v}, v.typ).Result()
}

// ExtendLifetime extends value lifetime without an explicit use.
func (b *Block) ExtendLifetime(v *Value) *Inst {
	return b.add(ExtendLifetime, Aux{}, []*Value{v})
}

// ---- memory ----

// AllocStack allocates local storage for type t and yields its address.
func (b *Block) AllocStack(t Type, attrs ...string) *Value {
	return b.add(AllocStack, Aux{Type: t, Attrs: attrs}, nil, t.Address()).Result()
}

// AllocStackFor allocates local stack storage attributed to a named source binding.
func (b *Block) AllocStackFor(t Type, name, kind string) *Value {
	return b.add(AllocStack, Aux{Type: t, Name: name, Attrs: []string{kind}}, nil,
		t.Address()).Result()
}

// DeallocStack deallocates stack storage.
func (b *Block) DeallocStack(addr *Value) *Inst {
	return b.add(DeallocStack, Aux{}, []*Value{addr})
}

// AllocRef allocates an instance of a class.
func (b *Block) AllocRef(t Type) *Value {
	return b.add(AllocRef, Aux{Type: t}, nil, t.Object()).Result()
}

// DeallocRef frees a class instance.
func (b *Block) DeallocRef(v *Value) *Inst {
	return b.add(DeallocRef, Aux{}, []*Value{v})
}

// AllocBox allocates a heap box for a mutable variable.
func (b *Block) AllocBox(elem Type, name string, attrs ...string) *Value {
	boxed := Box(elem.Formal())
	return b.add(AllocBox, Aux{Type: boxed, Name: name, Attrs: attrs}, nil,
		boxed).Result()
}

// AllocBoxOf allocates a heap box of the box type boxed.
func (b *Block) AllocBoxOf(boxed Type, name string, attrs ...string) *Value {
	return b.add(AllocBox, Aux{Type: boxed, Name: name, Attrs: attrs}, nil,
		boxed).Result()
}

// ProjectBox yields the address of what a box holds.
func (b *Block) ProjectBox(box *Value, field int, t Type) *Value {
	return b.add(ProjectBox, Aux{Int: int64(field)}, []*Value{box}, t.Address()).Result()
}

// Load reads a value from an address with ownership attr (copy, take, trivial).
func (b *Block) Load(addr *Value, attr string) *Value {
	return b.add(Load, Aux{Attrs: []string{attr}}, []*Value{addr},
		addr.typ.Object()).Result()
}

// Store writes a value to an address with initialization attr (init, assign).
func (b *Block) Store(v, addr *Value, attr string) *Inst {
	return b.add(Store, Aux{Attrs: []string{attr}}, []*Value{v, addr})
}

// Assign stores to an address that already holds a value.
func (b *Block) Assign(v, addr *Value) *Inst {
	return b.add(Assign, Aux{}, []*Value{v, addr})
}

// CopyAddr copies between source and destination addresses.
func (b *Block) CopyAddr(src, dst *Value, attrs ...string) *Inst {
	return b.add(CopyAddr, Aux{Attrs: attrs}, []*Value{src, dst})
}

// DestroyAddr destroys the value stored at an address.
func (b *Block) DestroyAddr(addr *Value) *Inst {
	return b.add(DestroyAddr, Aux{}, []*Value{addr})
}

// BeginAccess opens an exclusive or shared access scope to memory.
func (b *Block) BeginAccess(addr *Value, attrs ...string) *Value {
	return b.add(BeginAccess, Aux{Attrs: attrs}, []*Value{addr}, addr.typ).Result()
}

// EndAccess closes an access scope.
func (b *Block) EndAccess(v *Value) *Inst {
	return b.add(EndAccess, Aux{}, []*Value{v})
}

// MarkUninitialized marks storage requiring definite initialization analysis.
func (b *Block) MarkUninitialized(addr *Value, kind string) *Value {
	return b.add(MarkUninitialized, Aux{Attrs: []string{kind}},
		[]*Value{addr}, addr.typ).Result()
}

// ---- aggregates ----

// Struct builds a struct value from field operands in declaration order.
func (b *Block) Struct(t Type, fields ...*Value) *Value {
	return b.add(Struct, Aux{Type: t}, fields, t.Object()).Result()
}

// StructExtract reads a field from a struct value.
func (b *Block) StructExtract(v *Value, member string, t Type) *Value {
	return b.add(StructExtract, Aux{Member: member}, []*Value{v}, t.Object()).Result()
}

// StructElementAddr yields the address of a struct field.
func (b *Block) StructElementAddr(addr *Value, member string, t Type) *Value {
	return b.add(StructElementAddr, Aux{Member: member}, []*Value{addr},
		t.Address()).Result()
}

// Tuple builds a tuple value.
func (b *Block) Tuple(t Type, elems ...*Value) *Value {
	return b.add(Tuple, Aux{Type: t}, elems, t.Object()).Result()
}

// TupleExtract reads a tuple element by position.
func (b *Block) TupleExtract(v *Value, index int, t Type) *Value {
	return b.add(TupleExtract, Aux{Int: int64(index)}, []*Value{v}, t.Object()).Result()
}

// DestructureTuple decomposes a tuple into all constituent elements.
func (b *Block) DestructureTuple(v *Value, elems ...Type) []*Value {
	return b.add(DestructureTuple, Aux{}, []*Value{v}, elems...).results
}

// Enum builds an enum value from a case and optional payload.
func (b *Block) Enum(t Type, member string, payload *Value) *Value {
	var args []*Value
	if payload != nil {
		args = []*Value{payload}
	}
	return b.add(Enum, Aux{Type: t, Member: member}, args, t.Object()).Result()
}

// UncheckedEnumData reads a case's payload without checking tag.
func (b *Block) UncheckedEnumData(v *Value, member string, t Type) *Value {
	return b.add(UncheckedEnumData, Aux{Member: member}, []*Value{v}, t.Object()).Result()
}

// ---- references and calls ----

// RefElementAddr yields the address of a stored property in a class instance.
func (b *Block) RefElementAddr(ref *Value, member string, t Type) *Value {
	return b.add(RefElementAddr, Aux{Member: member}, []*Value{ref},
		t.Address()).Result()
}

// FunctionRef creates a reference to a function.
func (b *Block) FunctionRef(fn *Func) *Value {
	t := Type{formal: fn.typ}
	return b.add(FunctionRef, Aux{Name: fn.name}, nil, t).Result()
}

// ClassMethod looks up a method in an instance vtable.
func (b *Block) ClassMethod(ref *Value, member string, t Type) *Value {
	return b.add(ClassMethod, Aux{Member: member}, []*Value{ref}, t).Result()
}

// WitnessMethod looks up a protocol requirement in a witness table.
func (b *Block) WitnessMethod(member string, t Type) *Value {
	return b.add(WitnessMethod, Aux{Member: member}, nil, t).Result()
}

// WitnessMethodOn looks up a requirement in an existential witness table.
func (b *Block) WitnessMethodOn(recv *Value, member string, t Type) *Value {
	return b.add(WitnessMethod, Aux{Member: member}, []*Value{recv}, t).Result()
}

// InitExistentialAddr opens an existential container to initialize concrete value storage.
func (b *Block) InitExistentialAddr(addr *Value, concrete Type, meta *Value) *Value {
	args := []*Value{addr}
	if meta != nil {
		args = append(args, meta)
	}
	return b.add(InitExistentialAddr, Aux{Type: concrete},
		args, concrete.Address()).Result()
}

// OpenExistentialAddr yields the address of the value inside an existential.
func (b *Block) OpenExistentialAddr(addr *Value, t Type) *Value {
	return b.add(OpenExistentialAddr, Aux{}, []*Value{addr}, t).Result()
}

// Apply calls a function value.
func (b *Block) Apply(callee *Value, result Type, args ...*Value) *Value {
	return b.add(Apply, Aux{}, append([]*Value{callee}, args...), result).Result()
}

// ThinToThickFunction converts a thin function to a thick function value.
func (b *Block) ThinToThickFunction(fn *Value, t Type) *Value {
	return b.add(ThinToThickFunction, Aux{Type: t}, []*Value{fn}, t).Result()
}

// PartialApply binds leading arguments to produce a thick closure value.
func (b *Block) PartialApply(callee *Value, t Type, args ...*Value) *Value {
	return b.add(PartialApply, Aux{Attrs: []string{"callee_guaranteed"}},
		append([]*Value{callee}, args...), t).Result()
}

// GlobalAddr is the address of a module-level variable's storage.
func (b *Block) GlobalAddr(g *Global) *Value {
	return b.add(GlobalAddr, Aux{Type: g.Type(), Name: g.Name()}, nil, g.Type().Address()).Result()
}

// Metatype emits a thin metatype for instance.
func (b *Block) Metatype(instance Type) *Value {
	t := ThinMetatype(instance.Formal())
	return b.add(Metatype, Aux{Type: t}, nil, t).Result()
}

// ClassMetatype emits runtime class metadata via an accessor function.
func (b *Block) ClassMetatype(instance Type, accessor string) *Value {
	t := ThickMetatype(instance.Formal())
	return b.add(Metatype, Aux{Type: t, Name: accessor}, nil, t).Result()
}

// AddressToPointer casts an address to Builtin.RawPointer.
func (b *Block) AddressToPointer(addr *Value, t Type) *Value {
	return b.add(AddressToPointer, Aux{Type: t}, []*Value{addr}, t).Result()
}

// PointerToAddress casts a Builtin.RawPointer to an address of type t.
func (b *Block) PointerToAddress(p *Value, t Type) *Value {
	return b.add(PointerToAddress, Aux{Type: t}, []*Value{p}, t).Result()
}

// IndexAddr yields the address of an element at index offset from base.
func (b *Block) IndexAddr(base, index *Value) *Value {
	return b.add(IndexAddr, Aux{}, []*Value{base, index}, base.Type()).Result()
}

// TypeMetadata emits type metadata as an untyped pointer.
func (b *Block) TypeMetadata(instance Type, accessor string) *Value {
	return b.add(Metatype, Aux{Type: ThickMetatype(instance.Formal()), Name: accessor},
		nil, Object(BuiltinRawPointer)).Result()
}

// MetadataGlobal emits a pointer to an exported type metadata record at the specified offset.
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

// FloatLiteral emits a literal of a builtin floating-point type using raw IEEE bit representation.
func (b *Block) FloatLiteral(t Type, bits int64) *Value {
	return b.add(FloatLiteral, Aux{Type: t, Int: bits}, nil, t).Result()
}

// StringLiteral is a literal of Builtin.RawPointer, with an encoding.
func (b *Block) StringLiteral(s string, encoding string) *Value {
	return b.add(StringLiteral, Aux{Text: s, Attrs: []string{encoding}}, nil,
		Object(BuiltinRawPointer)).Result()
}

// ---- builtins ----

// Builtin invokes a compiler builtin instruction by name.
func (b *Block) Builtin(name string, result Type, args ...*Value) *Value {
	return b.add(BuiltinCall, Aux{Name: name}, args, result).Result()
}

// CondFail traps if cond evaluates to true.
func (b *Block) CondFail(cond *Value, message string) *Inst {
	return b.add(CondFail, Aux{Text: message}, []*Value{cond})
}

// ---- debug ----

// DebugValue emits debug variable attribution.
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

// TryApply invokes a throwing function, branching to normal or error successor blocks.
func (b *Block) TryApply(callee *Value, normal, failed *Block, args ...*Value) *Inst {
	return b.add(TryApply, Aux{Cases: []Case{
		{Member: "normal", Dest: normal},
		{Member: "error", Dest: failed},
	}}, append([]*Value{callee}, args...))
}

// SwitchEnum branches based on the active enum case, passing payloads to destination blocks.
func (b *Block) SwitchEnum(v *Value, cases ...Case) *Inst {
	return b.add(SwitchEnum, Aux{Cases: cases}, []*Value{v})
}

// Return exits the current function returning v.
func (b *Block) Return(v *Value) *Inst {
	return b.add(Return, Aux{}, []*Value{v})
}

// Throw exits along the error edge returning error value v.
func (b *Block) Throw(v *Value) *Inst {
	return b.add(Throw, Aux{}, []*Value{v})
}

// Unreachable asserts control flow cannot reach this point.
func (b *Block) Unreachable() *Inst {
	return b.add(Unreachable, Aux{}, nil)
}
