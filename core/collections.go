package core

import (
	"github.com/vertex-language/vsc/stdlib"
	"github.com/vertex-language/vsc/types"
)

// The methods and properties of Array, Dictionary and Set that the
// runtime implements.
//
// Each is a call into runtime/collections.cpp, and what the table says is
// how the call is made: which operands the runtime function takes, in
// order, and where each comes from. A value of a type the runtime cannot
// see into crosses by address with its type's metadata after it, so the
// same function serves every element type.

// An OperandKind is where one operand of a collection call comes from.
type OperandKind int

const (
	// OpReceiverSlot is the address of the storage the receiver lives in,
	// which a mutating call writes through.
	OpReceiverSlot OperandKind = iota
	// OpReceiver is the receiver's value, borrowed for the call.
	OpReceiver
	// OpArgTake is an argument by address, handed over to the runtime.
	OpArgTake
	// OpArgBorrow is an argument by address, lent for the call.
	OpArgBorrow
	// OpArgValue is an argument in its own register: an index, an Array.
	OpArgValue
	// OpOut is the address the result is written to.
	OpOut
	// OpMeta is the metadata of Type.
	OpMeta
	// OpReceiverAddress is the receiver's value, put in memory and lent
	// by address: a struct wider than the registers a C call takes it in.
	OpReceiverAddress
)

// An Operand is one operand of a collection call.
type Operand struct {
	Kind OperandKind
	// Arg is which of the call's arguments, for the argument kinds.
	Arg int
	// Type is the type whose metadata OpMeta passes.
	Type types.Type
}

// A Method is a collection method or property the runtime implements.
type Method struct {
	Symbol string
	// Params are the parameters as the source writes them: labels and
	// types, for the checker.
	Params []*types.Param
	// Result is what the call answers; Void for nothing.
	Result types.Type
	// Mutating says the receiver has to be a variable, whose storage the
	// call writes through.
	Mutating bool
	// ResultOut says the result is written to the OpOut operand rather
	// than answered in a register.
	ResultOut bool
	Operands  []Operand
}

// Signature is the method's type as the checker sees it.
func (m Method) Signature() *types.Signature {
	return &types.Signature{Params: m.Params, Results: m.Result}
}

var (
	voidType = types.Typ[types.Void]
	intType  = types.Typ[types.Int]
	boolType = types.Typ[types.Bool]
)

func param(label string, t types.Type) *types.Param {
	return &types.Param{Name: label, Label: label, Type: t}
}

func meta(t types.Type) Operand { return Operand{Kind: OpMeta, Type: t} }

// RuntimeHashable reports whether the runtime can hash and compare values
// of a type: the integers, Bool, Float, Double and String itself, and any
// other Hashable type through its conformance. A key, a Set element, and
// an element Array.contains looks for have to be one.
func RuntimeHashable(t types.Type) bool {
	if t == nil {
		return false
	}
	b, ok := t.Underlying().(*types.Basic)
	if !ok {
		return types.ConformsToNamed(t, "Hashable")
	}
	switch b.Kind() {
	case types.Int, types.Int8, types.Int16, types.Int32, types.Int64,
		types.UInt, types.UInt8, types.UInt16, types.UInt32, types.UInt64,
		types.Bool, types.Float, types.Double, types.String:
		return true
	}
	return false
}

// ArrayEqual is `==` on two arrays, for an element type the runtime
// compares, or false for one it does not.
func ArrayEqual(a *types.Array) (Method, bool) {
	if !RuntimeHashable(a.Elem) {
		return Method{}, false
	}
	return Method{Symbol: stdlib.ArrayEqual, Params: []*types.Param{param("", a)}, Result: boolType,
		Operands: []Operand{{Kind: OpReceiver}, {Kind: OpArgValue, Arg: 0}, meta(a.Elem)}}, true
}

// LowerCollectionMethod is the runtime method a call names on a
// collection, chosen by its name and argument labels ("" for an
// unlabelled argument), or reports that there is none.
func LowerCollectionMethod(recv types.Type, name string, labels []string) (Method, bool) {
	if recv == nil {
		return Method{}, false
	}
	is := func(want ...string) bool {
		if len(labels) != len(want) {
			return false
		}
		for i := range want {
			if labels[i] != want[i] {
				return false
			}
		}
		return true
	}
	switch u := recv.Underlying().(type) {
	case *types.Basic:
		if u.Kind() != types.String {
			return Method{}, false
		}
		str := types.Typ[types.String]
		switch {
		case (name == "hasPrefix" || name == "hasSuffix") && is(""), name == "starts" && is("with"):
			symbol := stdlib.StringHasPrefix
			if name == "hasSuffix" {
				symbol = stdlib.StringHasSuffix
			}
			return Method{Symbol: symbol, Params: []*types.Param{param(labels[0], str)}, Result: boolType,
				Operands: []Operand{{Kind: OpReceiver}, {Kind: OpArgValue, Arg: 0}}}, true
		case name == "append" && (is("") || is("contentsOf")):
			return Method{Symbol: stdlib.StringAppend, Params: []*types.Param{param(labels[0], str)},
				Result: voidType, Mutating: true,
				Operands: []Operand{{Kind: OpReceiverSlot}, {Kind: OpArgValue, Arg: 0}}}, true
		}
	case *types.Array:
		elem := u.Elem
		switch {
		case name == "append" && is(""):
			return Method{Symbol: stdlib.ArrayAppend, Params: []*types.Param{param("", elem)},
				Result: voidType, Mutating: true,
				Operands: []Operand{{Kind: OpReceiverSlot}, {Kind: OpArgTake, Arg: 0}, meta(elem)}}, true
		case name == "append" && is("contentsOf"):
			return Method{Symbol: stdlib.ArrayAppendContents,
				Params: []*types.Param{param("contentsOf", recv)}, Result: voidType, Mutating: true,
				Operands: []Operand{{Kind: OpReceiverSlot}, {Kind: OpArgValue, Arg: 0}, meta(elem)}}, true
		case name == "insert" && is("", "at"):
			return Method{Symbol: stdlib.ArrayInsert,
				Params: []*types.Param{param("", elem), param("at", intType)}, Result: voidType, Mutating: true,
				Operands: []Operand{{Kind: OpReceiverSlot}, {Kind: OpArgTake, Arg: 0},
					{Kind: OpArgValue, Arg: 1}, meta(elem)}}, true
		case name == "remove" && is("at"):
			return Method{Symbol: stdlib.ArrayRemoveAt, Params: []*types.Param{param("at", intType)},
				Result: elem, Mutating: true, ResultOut: true,
				Operands: []Operand{{Kind: OpReceiverSlot}, {Kind: OpArgValue, Arg: 0}, {Kind: OpOut}, meta(elem)}}, true
		case name == "removeLast" && is():
			return Method{Symbol: stdlib.ArrayRemoveLast, Result: elem, Mutating: true, ResultOut: true,
				Operands: []Operand{{Kind: OpReceiverSlot}, {Kind: OpOut}, meta(elem)}}, true
		case name == "removeAll" && is():
			return Method{Symbol: stdlib.ArrayRemoveAll, Result: voidType, Mutating: true,
				Operands: []Operand{{Kind: OpReceiverSlot}, meta(elem)}}, true
		case name == "contains" && is("") && RuntimeHashable(elem):
			return Method{Symbol: stdlib.ArrayContains, Params: []*types.Param{param("", elem)}, Result: boolType,
				Operands: []Operand{{Kind: OpReceiver}, {Kind: OpArgBorrow, Arg: 0}, meta(elem)}}, true
		}
	case *types.Dictionary:
		valueOpt := &types.Optional{Wrapped: u.Value}
		switch {
		case name == "removeValue" && is("forKey") && RuntimeHashable(u.Key):
			return Method{Symbol: stdlib.DictionaryRemove, Params: []*types.Param{param("forKey", u.Key)},
				Result: valueOpt, Mutating: true, ResultOut: true,
				Operands: []Operand{{Kind: OpReceiverSlot}, {Kind: OpArgBorrow, Arg: 0}, {Kind: OpOut},
					meta(u.Key), meta(valueOpt)}}, true
		}
	case *types.Set:
		elem := u.Elem
		if !RuntimeHashable(elem) {
			return Method{}, false
		}
		switch {
		case name == "insert" && is(""):
			return Method{Symbol: stdlib.SetInsert, Params: []*types.Param{param("", elem)},
				Result: voidType, Mutating: true,
				Operands: []Operand{{Kind: OpReceiverSlot}, {Kind: OpArgBorrow, Arg: 0}, meta(elem)}}, true
		case name == "contains" && is(""):
			return Method{Symbol: stdlib.SetContains, Params: []*types.Param{param("", elem)}, Result: boolType,
				Operands: []Operand{{Kind: OpReceiver}, {Kind: OpArgBorrow, Arg: 0}, meta(elem)}}, true
		case name == "remove" && is(""):
			opt := &types.Optional{Wrapped: elem}
			return Method{Symbol: stdlib.SetRemove, Params: []*types.Param{param("", elem)},
				Result: opt, Mutating: true, ResultOut: true,
				Operands: []Operand{{Kind: OpReceiverSlot}, {Kind: OpArgBorrow, Arg: 0}, {Kind: OpOut}, meta(opt)}}, true
		}
	}
	return Method{}, false
}

// LowerCollectionProperty is the runtime property a member of a
// collection names, or reports that there is none. Array's count and
// isEmpty are LowerMember's.
func LowerCollectionProperty(recv types.Type, name string) (Method, bool) {
	if recv == nil {
		return Method{}, false
	}
	switch u := recv.Underlying().(type) {
	case *types.Array:
		opt := &types.Optional{Wrapped: u.Elem}
		switch name {
		case "first":
			return Method{Symbol: stdlib.ArrayFirst, Result: opt, ResultOut: true,
				Operands: []Operand{{Kind: OpReceiver}, {Kind: OpOut}, meta(opt)}}, true
		case "last":
			return Method{Symbol: stdlib.ArrayLast, Result: opt, ResultOut: true,
				Operands: []Operand{{Kind: OpReceiver}, {Kind: OpOut}, meta(opt)}}, true
		}
	case *types.Dictionary, *types.Set:
		switch name {
		case "count":
			return Method{Symbol: stdlib.HashTableCount, Result: intType,
				Operands: []Operand{{Kind: OpReceiver}}}, true
		case "isEmpty":
			return Method{Symbol: stdlib.HashTableIsEmpty, Result: boolType,
				Operands: []Operand{{Kind: OpReceiver}}}, true
		}
	}
	return Method{}, false
}

// DictionaryGet is `d[key]`, and DictionarySet `d[key] = value` with the
// value an Optional; both only where the runtime hashes the key type.
func DictionaryGet(d *types.Dictionary) (Method, bool) {
	if !RuntimeHashable(d.Key) {
		return Method{}, false
	}
	opt := &types.Optional{Wrapped: d.Value}
	return Method{Symbol: stdlib.DictionaryGet, Params: []*types.Param{param("", d.Key)},
		Result: opt, ResultOut: true,
		Operands: []Operand{{Kind: OpReceiver}, {Kind: OpArgBorrow, Arg: 0}, {Kind: OpOut}, meta(opt)}}, true
}

// DictionaryGetDefault is `d[key, default: value]`: the value for key, or
// the default. The default is evaluated and handed over whether it is
// used or not, where Swift's is an autoclosure evaluated only when used.
func DictionaryGetDefault(d *types.Dictionary) (Method, bool) {
	if !RuntimeHashable(d.Key) {
		return Method{}, false
	}
	return Method{Symbol: stdlib.DictionaryGetDefault,
		Params: []*types.Param{param("", d.Key), param("default", d.Value)},
		Result: d.Value, ResultOut: true,
		Operands: []Operand{{Kind: OpReceiver}, {Kind: OpArgBorrow, Arg: 0}, {Kind: OpArgTake, Arg: 1},
			{Kind: OpOut}, meta(d.Value)}}, true
}

func DictionarySet(d *types.Dictionary) (Method, bool) {
	if !RuntimeHashable(d.Key) {
		return Method{}, false
	}
	opt := &types.Optional{Wrapped: d.Value}
	return Method{Symbol: stdlib.DictionarySet, Params: []*types.Param{param("", d.Key), param("", opt)},
		Result: voidType, Mutating: true,
		Operands: []Operand{{Kind: OpReceiverSlot}, {Kind: OpArgBorrow, Arg: 0}, {Kind: OpArgTake, Arg: 1},
			meta(d.Key), meta(opt)}}, true
}

// ArraySet is `a[index] = value`.
func ArraySet(a *types.Array) Method {
	return Method{Symbol: stdlib.ArrayAssign, Params: []*types.Param{param("", intType), param("", a.Elem)},
		Result: voidType, Mutating: true,
		Operands: []Operand{{Kind: OpReceiverSlot}, {Kind: OpArgValue, Arg: 0}, {Kind: OpArgTake, Arg: 1},
			meta(a.Elem)}}
}

// HasherInit is `Hasher()`: the runtime seeds one in place.
func HasherInit(hasher types.Type) Method {
	return Method{Symbol: stdlib.HasherInit, Result: hasher, ResultOut: true,
		Operands: []Operand{{Kind: OpOut}}}
}

// HasherCombine is `hasher.combine(value)` for a value of this type: the
// runtime hashes one the core declares itself, and anything else through
// its Hashable conformance.
func HasherCombine(value types.Type) Method {
	return Method{Symbol: stdlib.HasherCombine, Params: []*types.Param{param("", value)}, Result: voidType,
		Mutating: true, Operands: []Operand{{Kind: OpReceiverSlot}, {Kind: OpArgBorrow, Arg: 0}, meta(value)}}
}

// HasherFinalize is `hasher.finalize()`.
func HasherFinalize() Method {
	return Method{Symbol: stdlib.HasherFinalize, Result: intType,
		Operands: []Operand{{Kind: OpReceiverAddress}}}
}

// ValuesEqual is `==` on two values of any type the runtime compares --
// the core's own, Optionals and Arrays of them, and Equatable types through
// their conformance -- lent by address with the type's metadata.
func ValuesEqual(t types.Type) Method {
	return Method{Symbol: stdlib.ValuesEqual, Params: []*types.Param{param("", t), param("", t)}, Result: boolType,
		Operands: []Operand{{Kind: OpArgBorrow, Arg: 0}, {Kind: OpArgBorrow, Arg: 1}, meta(t)}}
}
