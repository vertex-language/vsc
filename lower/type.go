package lower

import (
	"github.com/vertex-language/ir"
	"github.com/vertex-language/vsc/internal/sil"
	"github.com/vertex-language/vsc/stdlib"
	"github.com/vertex-language/vsc/types"
)

// repr specifies the register representation and optional bit width/sign for a lowered SIL value.
type repr struct {
	reg    ir.RegType // storage register
	width  uint       // non-zero if narrower than register (8 or 16 bits)
	signed bool       // true if narrow value should be sign-extended
}

func (r repr) narrow() bool { return r.width != 0 }

// machine returns the low-level register representation for a SIL type.
func machine(t sil.Type) (repr, bool) {
	if !t.IsValid() {
		return repr{}, false
	}
	if t.IsAddress() {
		return repr{reg: ir.TypePtr}, true
	}
	return machineOf(t.Formal())
}

func machineOf(t types.Type) (repr, bool) {
	switch t := t.(type) {
	case *sil.Builtin:
		return builtinRepr(t.Name())
	// A box is a reference to the heap object holding a variable.
	case *sil.BoxType:
		return repr{reg: ir.TypePtr}, true
	case *types.Basic:
		return basicRepr(t.Kind())
	case *types.Named:
		return machineOf(t.Underlying())
	// Specialized generic instances use their underlying struct/nominal layout.
	case *types.GenericInstance:
		return machineOf(t.Underlying())
	// Function signatures lower to code address pointers.
	case *types.Signature:
		// Not one register: the code and its context, two words. See
		// funcWords.
		return repr{}, false
	// An Optional without an extra tag byte uses spare payload representation (null pointer).
	case *types.Optional:
		if _, tagged := optionalImage(t); tagged {
			return repr{}, false
		}
		// An optional of a value of no bytes is its tag and nothing else.
		if emptyOptional(t) {
			return repr{reg: ir.TypeI32, width: 8}, true
		}
		// An optional of a plain enum is the enum's own tag, with nil one
		// past the last case.
		if _, ok := tagOptional(t); ok {
			return machineOf(t.Wrapped)
		}
		if r, ok := machineOf(t.Wrapped); ok && r.reg == ir.TypePtr {
			return r, true
		}
		return repr{}, false
	case *types.Class:
		// A class value is the reference, never the object.
		return repr{reg: ir.TypePtr}, true
	// Raw and typed pointers lower to IR pointer registers.
	case *types.Pointer:
		return repr{reg: ir.TypePtr}, true
	// Collection types lower to runtime storage reference pointers.
	case *types.Array, *types.Dictionary, *types.Set:
		return repr{reg: ir.TypePtr}, true
	case *types.Enum:
		// Payload enum fitting into a single word lowers as i64.
		if hasPayload(t) {
			image, ok := enumImage(t)
			if !ok || len(image.Fields) != 1 {
				return repr{}, false
			}
			return repr{reg: ir.TypeI64}, true
		}
		// Payloadless enums lower to their discriminator tag integer.
		if len(t.Cases) <= 1 {
			return repr{reg: ir.TypeI32, width: 8}, true
		}
		size := types.Sizeof(t, types.DefaultTarget64)
		switch {
		case size <= 1:
			return repr{reg: ir.TypeI32, width: 8}, true
		case size <= 2:
			return repr{reg: ir.TypeI32, width: 16}, true
		case size <= 4:
			return repr{reg: ir.TypeI32}, true
		}
		return repr{reg: ir.TypeI64}, true
	case *types.Struct:
		// Single-field wrapper structs (e.g. Int, Bool) unwrap to their field's
		// representation -- counting only fields that hold bytes, since a
		// field of no bytes travels in nothing. See appendLeaves.
		var only types.Type
		for _, f := range t.Fields {
			if f == nil || f.Type == nil {
				return repr{}, false
			}
			if types.Sizeof(f.Type, types.DefaultTarget64) == 0 {
				continue
			}
			if only != nil {
				return repr{}, false
			}
			only = f.Type
		}
		if only != nil {
			return machineOf(only)
		}
		return repr{}, false
	case *types.Tuple:
		// The same for a tuple: one element that holds bytes is that element.
		var only types.Type
		for _, e := range t.Elements {
			if e == nil || e.Type == nil {
				return repr{}, false
			}
			if types.Sizeof(e.Type, types.DefaultTarget64) == 0 {
				continue
			}
			if only != nil {
				return repr{}, false
			}
			only = e.Type
		}
		if only != nil {
			return machineOf(only)
		}
		return repr{}, false
	case *sil.FuncType:
		if t.Convention == sil.Thick {
			return repr{}, false
		}
		return repr{reg: ir.TypePtr}, true
	case *sil.MetatypeType:
		return repr{reg: ir.TypePtr}, true
	}
	return repr{}, false
}

func basicRepr(k types.BasicKind) (repr, bool) {
	switch k {
	case types.Bool:
		return repr{reg: ir.TypeI1}, true
	case types.Int, types.Int64:
		return repr{reg: ir.TypeI64, signed: true}, true
	case types.UInt, types.UInt64:
		return repr{reg: ir.TypeI64}, true
	case types.Int32:
		return repr{reg: ir.TypeI32, signed: true}, true
	case types.UInt32:
		return repr{reg: ir.TypeI32}, true
	case types.Int8:
		return repr{reg: ir.TypeI32, width: 8, signed: true}, true
	case types.UInt8:
		return repr{reg: ir.TypeI32, width: 8}, true
	case types.Int16:
		return repr{reg: ir.TypeI32, width: 16, signed: true}, true
	case types.UInt16:
		return repr{reg: ir.TypeI32, width: 16}, true
	case types.Float:
		return repr{reg: ir.TypeF32}, true
	case types.Double:
		return repr{reg: ir.TypeF64}, true
	}
	return repr{}, false
}

func builtinRepr(name string) (repr, bool) {
	switch name {
	case "Int1":
		return repr{reg: ir.TypeI1}, true
	case "Int8":
		return repr{reg: ir.TypeI32, width: 8, signed: true}, true
	case "Int16":
		return repr{reg: ir.TypeI32, width: 16, signed: true}, true
	case "Int32":
		return repr{reg: ir.TypeI32, signed: true}, true
	case "Int64", "Word", "IntLiteral":
		return repr{reg: ir.TypeI64, signed: true}, true
	case "FPIEEE32":
		return repr{reg: ir.TypeF32}, true
	case "FPIEEE64":
		return repr{reg: ir.TypeF64}, true
	case "NativeObject", "RawPointer":
		return repr{reg: ir.TypePtr}, true
	}
	return repr{}, false
}

// whyNoRegister formats a diagnostic explaining why a type cannot fit into a machine register.
func whyNoRegister(t sil.Type) string {
	if o, ok := optionalOfType(t); ok {
		if _, tagged := optionalImage(o); !tagged {
			return t.String() + ": an optional whose payload has a representation " +
				"to spare, and this knows which one only for a reference"
		}
	}
	if _, ok := payloadEnumOf(t); ok {
		return t.String() + ": an enum whose cases carry values, which is the " +
			"payload beside the tag -- a layout this package does not compute " +
			"yet, so one is usable inside a function and cannot cross a call"
	}
	if st, ok := structOf(t); ok && len(st.Fields) > 1 {
		return t.String() + ": a struct wider than " + itoa(maxDirectWords*8) +
			" bytes, which Swift passes by address and this package does not lay out yet"
	}
	return t.String()
}

// spareStructOptional reports whether an optional wraps a struct that
// spares a representation, as Swift lays such an optional out: the struct's
// own words, nil all of them zero, told apart by one word the struct never
// holds zero in. It answers the struct and that word's offset.
func spareStructOptional(o *types.Optional) (*types.Struct, int64, bool) {
	if o == nil || o.Wrapped == nil {
		return nil, 0, false
	}
	st, ok := o.Wrapped.Underlying().(*types.Struct)
	if !ok || len(st.TypeParams) > 0 {
		return nil, 0, false
	}
	if types.Sizeof(o, types.DefaultTarget64) != types.Sizeof(st, types.DefaultTarget64) {
		return nil, 0, false
	}
	at, ok := neverZeroWord(st, 0)
	if !ok {
		return nil, 0, false
	}
	return st, at, true
}

// neverZeroWord is the offset of a word a value of the type never holds zero
// in: a reference, a String's object word, a function's code.
func neverZeroWord(t types.Type, base int64) (int64, bool) {
	switch u := t.Underlying().(type) {
	case *types.Class, *types.Array, *types.Dictionary, *types.Set, *types.Signature:
		return base, true
	case *types.Basic:
		if u.Kind() == types.String {
			return base + stdlib.StringObjectWord*8, true
		}
	case *types.Struct:
		for _, f := range u.Fields {
			if f == nil || f.Type == nil {
				return 0, false
			}
			off, ok := types.Offsetof(u, f.Name, types.DefaultTarget64)
			if !ok {
				return 0, false
			}
			if at, ok := neverZeroWord(f.Type, base+off); ok {
				return at, true
			}
		}
	}
	return 0, false
}

// structOf resolves the concrete struct layout representation of a SIL type.
func structOf(t sil.Type) (*types.Struct, bool) {
	if !t.IsValid() || t.IsAddress() {
		return nil, false
	}
	f := t.Formal()
	if f == nil {
		return nil, false
	}
	if o, ok := f.Underlying().(*types.Optional); ok {
		if isStringOptional(o) {
			return stringImage()
		}
		// An optional function is the function's two words, with none
		// its code pointer null.
		if isFuncOptional(o) {
			return funcWords, true
		}
		if st, _, ok := spareStructOptional(o); ok {
			return st, true
		}
		return optionalImage(o)
	}
	if tu, ok := f.Underlying().(*types.Tuple); ok {
		return tupleImage(tu)
	}
	if e, ok := f.Underlying().(*types.Enum); ok {
		return enumImage(e)
	}
	if b, ok := f.Underlying().(*types.Basic); ok && b.Kind() == types.String {
		return stringImage()
	}
	if _, ok := f.Underlying().(*types.Signature); ok {
		return funcWords, true
	}
	st, ok := f.Underlying().(*types.Struct)
	return st, ok
}

// funcWords is a function value's image: the code to call, and the context
// it is called with in the self register, as swiftc lays a thick function
// out. A function that captures nothing has a null context.
// isFuncOptional reports whether an optional wraps a function value, which
// has no tag byte: the empty case is a null code pointer.
func isFuncOptional(o *types.Optional) bool {
	if o == nil || o.Wrapped == nil {
		return false
	}
	_, ok := o.Wrapped.Underlying().(*types.Signature)
	return ok
}

// funcContext is the type of a function value's context: a reference, which
// a copy of the value retains and a destroy releases -- null, for a function
// that captures nothing, which retain and release ignore.
var funcContext = &types.Class{Name: "$vsc_closure_context"}

var funcWords = &types.Struct{
	Name: "function",
	Fields: []*types.Field{
		{Name: "code", Type: &types.Pointer{Elem: types.Typ[types.Void]}},
		{Name: "context", Type: funcContext},
	},
}

// optionalImage returns the synthesized aggregate layout for an Optional with an extra tag byte ({payload, UInt8}).
func optionalImage(o *types.Optional) (*types.Struct, bool) {
	if o == nil || o.Wrapped == nil {
		return nil, false
	}
	payload := types.Sizeof(o.Wrapped, types.DefaultTarget64)
	if payload <= 0 || types.Sizeof(o, types.DefaultTarget64) != payload+1 {
		return nil, false
	}
	return &types.Struct{
		Name: "Optional<" + o.Wrapped.String() + ">",
		Fields: []*types.Field{
			{Name: "some", Type: o.Wrapped},
			{Name: optionalTagField, Type: types.Typ[types.UInt8]},
		},
	}, true
}

// enumOptional is the payload enum an optional wraps. The optional is
// the enum's words and a tag byte at the enum's size, inside its last
// word where the enum leaves room -- as types.Sizeof lays it out, and as
// swiftc does.
func enumOptional(o *types.Optional) (*types.Enum, bool) {
	if o == nil || o.Wrapped == nil {
		return nil, false
	}
	e, ok := o.Wrapped.Underlying().(*types.Enum)
	if !ok || !hasPayload(e) {
		return nil, false
	}
	return e, true
}

// tagOptional is the enum an optional wraps where that enum carries no
// values and its tag has room to spare: the optional is the tag, and nil
// is the first value no case uses -- the number of cases. Measured from
// swiftc: for `enum K { case a, b, c }`, `K?.none` is 3 in K's one byte,
// and for ten cases it is 10.
func tagOptional(o *types.Optional) (*types.Enum, bool) {
	if o == nil || o.Wrapped == nil {
		return nil, false
	}
	e, ok := o.Wrapped.Underlying().(*types.Enum)
	if !ok || hasPayload(e) || len(e.Cases) < 2 {
		return nil, false
	}
	if types.Sizeof(o, types.DefaultTarget64) != types.Sizeof(e, types.DefaultTarget64) {
		return nil, false
	}
	return e, true
}

// emptyOptional reports whether an optional wraps a value of no bytes --
// a one-case enum, an empty struct -- and so is its tag and nothing else:
// zero where there is a value, one where there is none.
func emptyOptional(o *types.Optional) bool {
	return o != nil && o.Wrapped != nil &&
		types.Sizeof(o.Wrapped, types.DefaultTarget64) == 0 &&
		types.Sizeof(o, types.DefaultTarget64) == 1
}

// optionalTagField is the name of the tag byte field in a tagged Optional aggregate.
const optionalTagField = "tag"

// empty reports whether a type occupies zero bytes (Void, empty tuple, empty struct, Never, thin metatype).
func empty(t sil.Type) bool {
	if !t.IsValid() || t.IsAddress() {
		return false
	}
	switch f := t.Formal().(type) {
	case *types.Basic:
		return f.Kind() == types.Void || f.Kind() == types.Never
	case *types.Tuple:
		return len(f.Elements) == 0
	case *types.Struct:
		return len(f.Fields) == 0
	case *sil.MetatypeType:
		return f.Thin()
	}
	return false
}

// optionalOfType is the Optional a type is, if it is one.
func optionalOfType(t sil.Type) (*types.Optional, bool) {
	if !t.IsValid() || t.IsAddress() || t.Formal() == nil {
		return nil, false
	}
	o, ok := t.Formal().Underlying().(*types.Optional)
	return o, ok
}

// tupleImage synthesizes a struct layout representing a tuple's sequential elements.
func tupleImage(t *types.Tuple) (*types.Struct, bool) {
	if t == nil || len(t.Elements) == 0 {
		return nil, false
	}
	fields := make([]*types.Field, 0, len(t.Elements))
	for i, e := range t.Elements {
		if e == nil || e.Type == nil {
			return nil, false
		}
		name := e.Name
		if name == "" {
			name = itoa(i)
		}
		fields = append(fields, &types.Field{Name: name, Type: e.Type})
	}
	return &types.Struct{Name: t.String(), Fields: fields}, true
}
