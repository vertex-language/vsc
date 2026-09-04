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
	case *types.Class:
		// A class value is the reference, never the object.
		return repr{reg: ir.TypePtr}, true
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
func empty(t vil.Type) bool {
	if !t.IsValid() || t.IsAddress() {
		return false
	}
	switch f := t.Formal().(type) {
	case *types.Basic:
		return f.Kind() == types.Void || f.Kind() == types.Never
	case *types.Tuple:
		return len(f.Elements) == 0
	}
	return false
}
