package vil

// An Op is an instruction's mnemonic, spelled as Swift spells it.
// There is no separate enum: the name is the identity, so a reader
// with SIL open needs no translation and a new instruction is one
// constant.
type Op string

// The instructions. Grouped as SIL groups them, named as SIL names
// them.
//
// The set is wider than what can be emitted: an opcode with no
// builder method in builder.go is vocabulary rather than something a
// module can hold, and it arrives with its text form when a construct
// needs it. What can be built is what vil/text prints and what
// vil/text's form tests hold to SIL's spelling.
const (
	// Allocation and deallocation.
	AllocStack   Op = "alloc_stack"
	AllocBox     Op = "alloc_box"
	AllocRef     Op = "alloc_ref"
	DeallocStack Op = "dealloc_stack"
	DeallocBox   Op = "dealloc_box"
	DeallocRef   Op = "dealloc_ref"
	ProjectBox   Op = "project_box"

	// Memory.
	Load              Op = "load"
	Store             Op = "store"
	CopyAddr          Op = "copy_addr"
	Assign            Op = "assign"
	DestroyAddr       Op = "destroy_addr"
	BeginAccess       Op = "begin_access"
	EndAccess         Op = "end_access"
	MarkUninitialized Op = "mark_uninitialized"

	// Reference counting: what the ownership instructions become once
	// the ownership form has been lowered away. Everything above the
	// eliminator reasons in copy and destroy; everything below it
	// counts.
	StrongRetain  Op = "strong_retain"
	StrongRelease Op = "strong_release"

	// Ownership.
	CopyValue      Op = "copy_value"
	DestroyValue   Op = "destroy_value"
	BeginBorrow    Op = "begin_borrow"
	EndBorrow      Op = "end_borrow"
	MoveValue      Op = "move_value"
	ExtendLifetime Op = "extend_lifetime"
	EndLifetime    Op = "end_lifetime"
	MarkDependence Op = "mark_dependence"

	// Aggregates.
	Struct            Op = "struct"
	StructExtract     Op = "struct_extract"
	StructElementAddr Op = "struct_element_addr"
	Tuple             Op = "tuple"
	TupleExtract      Op = "tuple_extract"
	TupleElementAddr  Op = "tuple_element_addr"
	DestructureTuple  Op = "destructure_tuple"
	Enum              Op = "enum"
	UncheckedEnumData Op = "unchecked_enum_data"
	InitEnumDataAddr  Op = "init_enum_data_addr"
	InjectEnumAddr    Op = "inject_enum_addr"

	// References and dispatch.
	RefElementAddr Op = "ref_element_addr"
	ClassMethod    Op = "class_method"
	WitnessMethod  Op = "witness_method"
	FunctionRef    Op = "function_ref"
	PartialApply   Op = "partial_apply"
	Apply          Op = "apply"
	TryApply       Op = "try_apply"
	Metatype       Op = "metatype"
	ValueMetatype  Op = "value_metatype"

	// Existentials.
	InitExistentialAddr Op = "init_existential_addr"
	OpenExistentialAddr Op = "open_existential_addr"
	InitExistentialRef  Op = "init_existential_ref"
	OpenExistentialRef  Op = "open_existential_ref"
	AllocExistentialBox Op = "alloc_existential_box"

	// Literals and casts.
	IntegerLiteral          Op = "integer_literal"
	FloatLiteral            Op = "float_literal"
	StringLiteral           Op = "string_literal"
	UncheckedRefCast        Op = "unchecked_ref_cast"
	UncheckedTrivialBitCast Op = "unchecked_trivial_bit_cast"
	Upcast                  Op = "upcast"
	UncheckedOwnershipConv  Op = "unchecked_ownership_conversion"
	ConvertFunction         Op = "convert_function"
	ThinToThickFunction     Op = "thin_to_thick_function"

	// Terminators.
	Br          Op = "br"
	CondBr      Op = "cond_br"
	SwitchEnum  Op = "switch_enum"
	SwitchValue Op = "switch_value"
	Return      Op = "return"
	Throw       Op = "throw"
	Unreachable Op = "unreachable"
	Yield       Op = "yield"
	Unwind      Op = "unwind"

	// Builtins: the machine instructions an operator's declaration
	// stands for, and the trap a checked one needs.
	BuiltinCall Op = "builtin"
	CondFail    Op = "cond_fail"

	// Debug.
	DebugValue Op = "debug_value"
)

func (o Op) String() string { return string(o) }

// IsTerminator reports whether o ends a block. A block has exactly
// one, and it is its last instruction.
func (o Op) IsTerminator() bool {
	switch o {
	case Br, CondBr, SwitchEnum, SwitchValue, Return, Throw,
		Unreachable, Yield, Unwind, TryApply:
		return true
	}
	return false
}

// Consumes reports whether o takes ownership of the operand at index
// i — which is what makes an owned value's single consumption
// findable, and rule 1 checkable.
func (o Op) Consumes(i int) bool {
	switch o {
	case DestroyValue, Return, Throw, MoveValue, EndLifetime:
		return i == 0
	case Store, Assign:
		return i == 0 // the value, not the address
	case DeallocRef, DeallocBox:
		return i == 0
	// An aggregate takes ownership of what is put into it.
	case Enum, Struct, Tuple:
		return true
	// A call is the one instruction whose answer is not in the
	// opcode: whether an argument is consumed is what the callee's
	// parameter convention says, and only the callee's type knows.
	// ConsumesArgument is the question with the type in hand.
	case Apply, TryApply, PartialApply:
		return false
	}
	return false
}

// ConsumesArgument reports whether a call takes ownership of the
// argument at index i, given the callee's type. Operand zero of a
// call is the callee itself and is never consumed; the rest are the
// arguments, and each is consumed exactly when its parameter is
// declared @owned or @in.
func ConsumesArgument(callee *FuncType, i int) bool {
	if callee == nil || i == 0 {
		return false
	}
	i--
	if i >= len(callee.Params) {
		return false
	}
	switch callee.Params[i].Convention {
	case ParamOwned, ParamIn:
		return true
	}
	return false
}

// Borrows reports whether o opens a borrow scope that a matching
// end_borrow closes.
func (o Op) Borrows() bool { return o == BeginBorrow || o == BeginAccess }
