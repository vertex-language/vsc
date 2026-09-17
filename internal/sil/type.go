package sil

import (
	"strings"

	"github.com/vertex-language/vsc/types"
)

// A Type is a SIL type: a formal type and a flag indicating whether
// it is an object ($T) or an address ($*T).
type Type struct {
	formal types.Type
	addr   bool
}

// Object is the type of a value of t.
func Object(t types.Type) Type { return Type{formal: t} }

// Address is the type of the address of a t.
func Address(t types.Type) Type { return Type{formal: t, addr: true} }

func (t Type) Formal() types.Type { return t.formal }
func (t Type) IsAddress() bool    { return t.addr }

// Object returns t as a value type; Address returns it as an address.
func (t Type) Object() Type  { return Type{formal: t.formal} }
func (t Type) Address() Type { return Type{formal: t.formal, addr: true} }

// IsValid reports whether t names anything.
func (t Type) IsValid() bool { return t.formal != nil }

// Equal reports whether two SIL types are the same type.
func (t Type) Equal(u Type) bool {
	return t.addr == u.addr && types.Identical(t.formal, u.formal)
}

// String is the text form, `$T` or `$*T`, as SIL writes it.
func (t Type) String() string {
	if t.formal == nil {
		return "$<invalid>"
	}
	if t.addr {
		return "$*" + t.formal.String()
	}
	return "$" + t.formal.String()
}

// Trivial reports whether values of this type have trivial ownership (no retain/release needed).
func (t Type) Trivial() bool { return trivial(t.formal) }

func trivial(t types.Type) bool {
	if t == nil {
		return true
	}
	switch n := t.Underlying().(type) {
	case *BoxType:
		return false
	case *MetatypeType:
		return true
	case *FuncType:
		return n.Convention != Thick
	case *types.Basic:
		return n.Kind() != types.String
	case *Builtin:
		return n.name != "NativeObject"
	case *types.Metatype:
		return true
	case *types.Pointer:
		return true
	case *types.Signature:
		// A function value holds its context, which it owns.
		return false
	case *types.Struct:
		for _, f := range n.Fields {
			if !trivial(f.Type) {
				return false
			}
		}
		return true
	case *types.Tuple:
		for _, e := range n.Elements {
			if !trivial(e.Type) {
				return false
			}
		}
		return true
	case *types.Enum:
		for _, c := range n.Cases {
			if !trivial(c.AssociatedType) {
				return false
			}
		}
		return true
	case *types.Optional:
		return trivial(n.Wrapped)
	}
	return false
}

// A BoxType represents a heap box holding a mutable variable ({ var T }).
type BoxType struct{ elem types.Type }

// Box is the type of a box holding t.
func Box(t types.Type) Type { return Object(&BoxType{elem: t}) }

func (b *BoxType) Underlying() types.Type { return b }
func (b *BoxType) Elem() types.Type       { return b.elem }
func (b *BoxType) String() string {
	if b.elem == nil {
		return "{ var <invalid> }"
	}
	return "{ var " + b.elem.String() + " }"
}

// A MetatypeType represents a metatype value (@thin or @thick T.Type).
type MetatypeType struct {
	instance types.Type
	repr     string // "thin" or "thick"
}

// ThinMetatype is the metatype of a type known statically.
func ThinMetatype(instance types.Type) Type {
	return Object(&MetatypeType{instance: instance, repr: "thin"})
}

// ThickMetatype is the metatype of a type carried at runtime.
func ThickMetatype(instance types.Type) Type {
	return Object(&MetatypeType{instance: instance, repr: "thick"})
}

func (m *MetatypeType) Underlying() types.Type { return m }
func (m *MetatypeType) Instance() types.Type   { return m.instance }

// Thin reports whether the metatype carries no bits: the type is
// known statically, so the value is nothing at run time.
func (m *MetatypeType) Thin() bool { return m.repr == "thin" }
func (m *MetatypeType) String() string {
	if m.instance == nil {
		return "@" + m.repr + " <invalid>.Type"
	}
	return "@" + m.repr + " " + m.instance.String() + ".Type"
}

// A Builtin represents a low-level builtin type (e.g. Builtin.Int64, Builtin.RawPointer).
type Builtin struct{ name string }

func (b *Builtin) Underlying() types.Type { return b }
func (b *Builtin) String() string         { return "Builtin." + b.name }
func (b *Builtin) Name() string           { return b.name }

// Common builtin types.
var (
	BuiltinInt1       = &Builtin{"Int1"}
	BuiltinInt8       = &Builtin{"Int8"}
	BuiltinInt16      = &Builtin{"Int16"}
	BuiltinInt32      = &Builtin{"Int32"}
	BuiltinInt64      = &Builtin{"Int64"}
	BuiltinWord       = &Builtin{"Word"}
	BuiltinFPIEEE32   = &Builtin{"FPIEEE32"}
	BuiltinFPIEEE64   = &Builtin{"FPIEEE64"}
	BuiltinIntLiteral = &Builtin{"IntLiteral"}
	BuiltinNativeObj  = &Builtin{"NativeObject"}
	BuiltinRawPointer = &Builtin{"RawPointer"}
)

// A Convention specifies a function calling convention.
type Convention string

const (
	Thin        Convention = "thin"           // Top-level function with no context
	Thick       Convention = "thick"          // Closure/function value with context
	Method      Convention = "method"         // Method receiving self
	C           Convention = "c"              // Platform C calling convention
	ConvWitness Convention = "witness_method" // Protocol requirement implementation
)

// A ParamConvention specifies parameter ownership across a call boundary.
type ParamConvention string

const (
	ParamOwned        ParamConvention = "@owned"
	ParamGuaranteed   ParamConvention = "@guaranteed"
	ParamUnowned      ParamConvention = ""
	ParamIn           ParamConvention = "@in"
	ParamInGuaranteed ParamConvention = "@in_guaranteed"
	ParamInout        ParamConvention = "@inout"
)

// A ResultConvention is what a caller receives.
type ResultConvention string

const (
	ResultOwned   ResultConvention = "@owned"
	ResultUnowned ResultConvention = ""
	ResultOut     ResultConvention = "@out"
	ResultAutorel ResultConvention = "@autoreleased"
)

// A Param is one parameter of a lowered function type.
type Param struct {
	Type       Type
	Convention ParamConvention
}

// A Result is one result.
type Result struct {
	Type       Type
	Convention ResultConvention
}

// A FuncType represents a lowered SIL function type with calling conventions.
type FuncType struct {
	Convention Convention
	Witness    string // the protocol, for @convention(witness_method: P)
	Params     []Param
	Results    []Result
	ErrorType  Type // set where the function throws
	Async      bool
	YieldOnce  bool
	Yields     []Result
}

func (f *FuncType) Underlying() types.Type { return f }

// String is SIL's spelling of a lowered function type.
func (f *FuncType) String() string {
	var b strings.Builder
	if f.YieldOnce {
		b.WriteString("@yield_once ")
	}
	b.WriteString("@convention(")
	b.WriteString(string(f.Convention))
	if f.Convention == ConvWitness && f.Witness != "" {
		b.WriteString(": " + f.Witness)
	}
	b.WriteString(") ")
	if f.Async {
		b.WriteString("@async ")
	}

	b.WriteByte('(')
	for i, p := range f.Params {
		if i > 0 {
			b.WriteString(", ")
		}
		if p.Convention != ParamUnowned {
			b.WriteString(string(p.Convention) + " ")
		}
		b.WriteString(bare(p.Type))
	}
	b.WriteString(") -> ")

	if len(f.Yields) > 0 {
		for _, y := range f.Yields {
			b.WriteString("@yields ")
			if y.Convention != ResultUnowned {
				b.WriteString(string(y.Convention) + " ")
			}
			b.WriteString(bare(y.Type))
		}
		return b.String()
	}

	results := f.resultText()
	if f.ErrorType.IsValid() {
		b.WriteString("(" + results + ", @error " + bare(f.ErrorType) + ")")
		return b.String()
	}
	b.WriteString(results)
	return b.String()
}

func (f *FuncType) resultText() string {
	switch len(f.Results) {
	case 0:
		return "()"
	case 1:
		r := f.Results[0]
		if r.Convention != ResultUnowned {
			return string(r.Convention) + " " + bare(r.Type)
		}
		return bare(r.Type)
	}
	parts := make([]string, len(f.Results))
	for i, r := range f.Results {
		parts[i] = bare(r.Type)
	}
	return "(" + strings.Join(parts, ", ") + ")"
}

// bare returns the type string without the leading '$'.
func bare(t Type) string {
	s := t.String()
	return strings.TrimPrefix(s, "$")
}
