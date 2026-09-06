package lower

import (
	"github.com/vertex-language/ir"
	"github.com/vertex-language/vsc/types"
	"github.com/vertex-language/vsc/vil"
)

// A repr is how a VIL value is held once the Swift is gone.
type repr struct {
	reg ir.RegType // the register it lives in
	// width is the declared width in bits for an integer narrower than
	// its register: 8 or 16, and 0 for everything that fills its own
	// register. Arithmetic at a narrow width is normalized back into
	// range, which is the price of §2 having no i8 and no i16.
	width uint
	// signed says how a narrow value is extended back into its register.
	signed bool
}

func (r repr) narrow() bool { return r.width != 0 }

// machine gives the representation of a VIL type, or false if this
// package does not yet know one. An address is always a pointer; an
// object is a register only when Swift holds it in one.
func machine(t vil.Type) (repr, bool) {
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
	case *vil.Builtin:
		return builtinRepr(t.Name())
	case *types.Basic:
		return basicRepr(t.Kind())
	case *types.Named:
		return machineOf(t.Underlying())
	// A function value. Swift's is two words -- the code address and
	// the context a closure captured -- and this is one, because gen
	// refuses a closure that captures and a context that can only ever
	// be null is not information. The second word arrives with
	// partial_apply, and vil.trivial says the same thing from the
	// other side.
	case *types.Signature:
		return repr{reg: ir.TypePtr}, true
	case *types.Class:
		// A class value is the reference, never the object.
		return repr{reg: ir.TypePtr}, true
	case *types.Enum:
		// An enum with no associated values is its tag and nothing
		// else, so it is an integer of whatever width the tag needs.
		// One with a payload is the payload beside the tag, which is a
		// layout this package does not compute yet.
		for _, c := range t.Cases {
			if c != nil && c.AssociatedType != nil {
				return repr{}, false
			}
		}
		if len(t.Cases) <= 1 {
			// One case carries no information: there is nothing to
			// tell apart and nothing to store.
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
		// A struct of one field is that field. It is the same rule
		// the instructions follow — `struct` and `struct_extract`
		// both forward a single-field aggregate rather than build or
		// take apart anything — and it has to hold here too, or a
		// value that lowers inside a function has no type to be
		// passed to another one by.
		//
		// Int and Bool are this: a struct around one builtin. So is
		// every wrapper a program declares for the same reason, and
		// they lower alike because after the names are gone they are
		// alike.
		//
		// More than one field is memory, and this package does not
		// lay a struct out yet — so it says so rather than picking a
		// register and losing the rest.
		if len(t.Fields) == 1 && t.Fields[0] != nil {
			return machineOf(t.Fields[0].Type)
		}
		// A struct with no fields is not "no register yet"; it is no
		// register, the way Void is. empty() is what says so, and
		// every caller that asks this question asks that one first.
		return repr{}, false
	case *vil.FuncType:
		// A thin function is its entry point. A thick one is a pair
		// and does not fit in a register.
		if t.Convention == vil.Thick {
			return repr{}, false
		}
		return repr{reg: ir.TypePtr}, true
	case *vil.MetatypeType:
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

// empty reports whether a type holds nothing: Void, the empty tuple,
// and Never, which holds nothing because control never arrives. A
// value of one of these becomes no register at all.
// whyNoRegister says what a type is, and where the answer is known,
// why it does not fit in a register.
//
// A signature is where the reason matters most: a struct is usable
// inside a function long before it can cross a call boundary, and
// "no machine type" would leave the reader thinking the type is
// unknown rather than that its ABI is undecided.
func whyNoRegister(t vil.Type) string {
	if st, ok := structOf(t); ok && len(st.Fields) > 1 {
		return t.String() + ": a struct wider than " + itoa(maxDirectWords*8) +
			" bytes, which Swift passes by address and this package does not lay out yet"
	}
	return t.String()
}

// structOf is the struct a type is, seeing through the name it was
// declared under.
func structOf(t vil.Type) (*types.Struct, bool) {
	if !t.IsValid() || t.IsAddress() {
		return nil, false
	}
	f := t.Formal()
	if f == nil {
		return nil, false
	}
	st, ok := f.Underlying().(*types.Struct)
	return st, ok
}

func empty(t vil.Type) bool {
	if !t.IsValid() || t.IsAddress() {
		return false
	}
	switch f := t.Formal().(type) {
	case *types.Basic:
		return f.Kind() == types.Void || f.Kind() == types.Never
	case *types.Tuple:
		return len(f.Elements) == 0
	// A struct with no stored properties holds nothing, so there is
	// nothing to pass, return or keep in a register. Swift's `struct
	// S {}` is a real type with methods and a size of zero, and this
	// used to have no machine type for it at all -- which refused a
	// struct that conformed to a protocol and declared no storage,
	// among others.
	case *types.Struct:
		return len(f.Fields) == 0
	}
	return false
}
