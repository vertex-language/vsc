package vil

import (
	"strings"

	"github.com/vertex-language/vsc/types"
)

// A Type is a VIL type: a formal type, and whether this is a value of
// it or the address of one.
//
// SIL's two-line rule, kept exactly. `$T` is a value; `$*T` is the
// address of one. A loadable type moves as a value; an address-only
// type is worked with through its address, which is what alloc_stack,
// load, store and copy_addr are for.
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

// Equal reports whether two VIL types are the same type.
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

// Trivial reports whether a value of this type owns nothing, and so
// needs no copy, no destroy, and no ownership at all. An Int is
// trivial; a class reference is not.
func (t Type) Trivial() bool { return trivial(t.formal) }

func trivial(t types.Type) bool {
	if t == nil {
		return true
	}
	switch n := t.Underlying().(type) {
	case *BoxType:
		return false // an allocation is owned whatever it holds
	case *MetatypeType:
		return true // a type is not a value that owns anything
	// A thin function is a code address and nothing else. A thick one
	// carries the context a closure captured, and something owns it.
	case *FuncType:
		return n.Convention != Thick
	case *types.Basic:
		return n.Kind() != types.String
	case *Builtin:
		return n.name != "NativeObject"
	case *types.Metatype:
		return true
	// A function value owns its context, and while a closure that
	// captures is refused there is no context to own: what a signature
	// holds is a code address. lower relies on this from the other
	// side, representing one in a single register, and both change
	// together the day partial_apply is emitted.
	case *types.Signature:
		return true
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
	}
	return false
}

// A BoxType is a heap box holding one variable: `{ var Int }`. It is
// what a `var` is in raw VIL, and it is a type of its own rather than
// its contents — a box is an allocation, so something owns it even
// when what it holds owns nothing.
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

// A MetatypeType is a type as a value: `@thin Int.Type`.
//
// The representation is part of the type, as it is in SIL. A thin
// metatype is nothing at runtime — the type is known statically and
// the value carries no bits. A thick one carries the metadata a
// generic or an existential needs to find its witnesses.
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
func (m *MetatypeType) String() string {
	if m.instance == nil {
		return "@" + m.repr + " <invalid>.Type"
	}
	return "@" + m.repr + " " + m.instance.String() + ".Type"
}

// A Builtin is one of the types the IR has and the source language
// does not: Builtin.Int64, Builtin.NativeObject, Builtin.IntLiteral.
// They are where a lowered value stops being a Swift value and starts
// being a machine one, and they print as SIL prints them.
type Builtin struct{ name string }

func (b *Builtin) Underlying() types.Type { return b }
func (b *Builtin) String() string         { return "Builtin." + b.name }
func (b *Builtin) Name() string           { return b.name }

// The builtins used so far. More arrive as lowering needs them.
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

// A Convention is how a function is called: the shape of its entry,
// not what it does.
type Convention string

const (
	// Thin is a function with no context: a top-level function
	// referenced by name.
	Thin Convention = "thin"
	// Thick is a function value with a context: what a closure is.
	Thick Convention = "thick"
	// Method takes self as its last parameter.
	Method Convention = "method"
	// C is the platform's C calling convention.
	C Convention = "c"
	// ConvWitness is a protocol requirement's implementation; the
	// protocol is named in the text form.
	ConvWitness Convention = "witness_method"
)

// A ParamConvention is what a callee does with a parameter: the half
// of the ownership model that crosses a call boundary.
type ParamConvention string

const (
	// ParamOwned: the callee takes ownership and must consume it.
	ParamOwned ParamConvention = "@owned"
	// ParamGuaranteed: the caller keeps it alive across the call.
	ParamGuaranteed ParamConvention = "@guaranteed"
	// ParamUnowned: neither; the callee may not assume a lifetime.
	ParamUnowned ParamConvention = ""
	// ParamIn, ParamInGuaranteed: an address the callee reads, owned
	// or borrowed.
	ParamIn           ParamConvention = "@in"
	ParamInGuaranteed ParamConvention = "@in_guaranteed"
	// ParamInout: an address the callee reads and writes.
	ParamInout ParamConvention = "@inout"
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

// A FuncType is a lowered function type: `$@convention(thin) (@guaranteed
// Box) -> Int`. It is a types.Type so that a function value has a type
// like anything else, but it is not a type the source can write —
// lowering produced it from one that could.
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

// bare is a type without the leading '$': inside a function type the
// sigil is written once, at the front.
func bare(t Type) string {
	s := t.String()
	return strings.TrimPrefix(s, "$")
}
