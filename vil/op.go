package vil

// An Op is an instruction's mnemonic, spelled as Swift spells it.
// There is no separate enum: the name is the identity, so a reader
// with SIL open needs no translation and a new instruction is one
// constant.
type Op string

// The instructions. Grouped as SIL groups them, named as SIL names
// them. This is the set the first subset needs; the rest arrive under
// their own names when a construct requires them.
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
	DestroyAddr       Op = "destroy_addr"
	BeginAccess       Op = "begin_access"
	EndAccess         Op = "end_access"
	MarkUninitialized Op = "mark_uninitialized"

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
	case Store:
		return i == 0 // the value, not the address
	case DeallocRef, DeallocBox:
		return i == 0
	case Enum, Struct, Tuple, Apply, TryApply, PartialApply:
		return true // every operand is forwarded into the result
	}
	return false
}

// Borrows reports whether o opens a borrow scope that a matching
// end_borrow closes.
func (o Op) Borrows() bool { return o == BeginBorrow || o == BeginAccess }
