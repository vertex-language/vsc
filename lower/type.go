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
	// A generic type with its arguments given is the type those
	// arguments make of it: Box<Int32> is a struct holding an Int32,
	// and every question below is about that struct.
	case *types.GenericInstance:
		return machineOf(t.Underlying())
	// A function value. Swift's is two words -- the code address and
	// the context a closure captured -- and this is one, because gen
	// refuses a closure that captures and a context that can only ever
	// be null is not information. The second word arrives with
	// partial_apply, and vil.trivial says the same thing from the
	// other side.
	case *types.Signature:
		return repr{reg: ir.TypePtr}, true
	// An Optional with no tag byte of its own is its payload: the
	// empty case took one of the representations the payload does not
	// use, so the bytes are the payload's bytes and the register is
	// the payload's register. One with a tag byte is wider than a
	// register may be and goes through structOf, like any other
	// struct.
	case *types.Optional:
		if _, tagged := optionalImage(t); tagged {
			return repr{}, false
		}
		// No tag byte, so the empty case took one of the
		// representations the payload does not use -- and which one
		// is a fact about that type. A reference's is null, and a
		// null is a pointer like any other.
		//
		// A pointer's is null too, for the same reason and with the
		// same bytes.
		//
		// A Bool's is not. It leaves 254 of its 256 spare and Swift
		// takes the first, so `nil as Bool?` is the byte 2 -- which a
		// one-bit Bool cannot hold. Modelling that needs a Bool that
		// is a byte rather than a bit, which this compiler does not
		// have, so `Bool?` has no representation here rather than a
		// nearly-right one.
		if r, ok := machineOf(t.Wrapped); ok && r.reg == ir.TypePtr {
			return r, true
		}
		return repr{}, false
	case *types.Class:
		// A class value is the reference, never the object.
		return repr{reg: ir.TypePtr}, true
	// An unsafe pointer is an address, which is what a pointer
	// register holds. What it points at changes nothing here: the
	// element type decides what a read through it loads, and that is
	// a question asked where the read is.
	//
	// It follows that `UnsafePointer<T>?` needs no tag byte, because
	// null is a representation an address in use does not have --
	// which is what Swift does too, and why a nullable C pointer is
	// one word in both.
	case *types.Pointer:
		return repr{reg: ir.TypePtr}, true
	// An Array is one word too: a reference to the storage that holds
	// the elements. swiftc's own code for a function taking one reads
	// x0 and nothing else.
	case *types.Array:
		return repr{reg: ir.TypePtr}, true
	case *types.Enum:
		// An enum with no associated values is its tag and nothing
		// else, so it is an integer of whatever width the tag needs.
		//
		// One with a payload is the payload beside the tag, and its
		// memory image is words -- see enumImage. A multi-word one
		// crosses a call as those words, which directWords arranges
		// out of its leaves. A one-word one has a single leaf, so
		// directWords does not apply to it, and without a register
		// here it could not cross a call at all: it was usable inside
		// a function and had no machine type at a boundary.
		//
		// The word is what it already is. makePayloadEnum builds the
		// image and defines the result as the single value when there
		// is one, so saying i64 here says what the value has been all
		// along.
		if hasPayload(t) {
			image, ok := enumImage(t)
			if !ok || len(image.Fields) != 1 {
				return repr{}, false
			}
			return repr{reg: ir.TypeI64}, true
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
	if o, ok := f.Underlying().(*types.Optional); ok {
		return optionalImage(o)
	}
	// A tuple's bytes are a struct's bytes: elements in order, each
	// at the offset its alignment puts it. Swift passes one the way
	// it passes the struct with the same elements, so saying that
	// here is what lets a tuple cross a call at all.
	if tu, ok := f.Underlying().(*types.Tuple); ok {
		return tupleImage(tu)
	}
	// An enum whose cases carry values travels as its bytes: the
	// cases share the payload area, so there is no one list of fields
	// and what crosses a call is the words. See enumImage.
	if e, ok := f.Underlying().(*types.Enum); ok {
		return enumImage(e)
	}
	// A String is two words and travels as the struct of two words
	// is: swiftc's own code reads x0 and x1 for one. See string.go.
	if b, ok := f.Underlying().(*types.Basic); ok && b.Kind() == types.String {
		return stringImage()
	}
	st, ok := f.Underlying().(*types.Struct)
	return st, ok
}

// optionalImage is the struct an Optional's bytes look like, where it
// has bytes of its own.
//
// swiftc's own code for `takeOpt(7)` writes the payload at offset
// zero, a tag byte after it, and loads the eight bytes into x0; the
// callee reads that byte back and compares it with one. So `Int32?`
// is `{ Int32, UInt8 }` and is passed the way that struct is passed,
// and the image is the answer to every question the ABI asks about
// it.
//
// Not every Optional has a tag byte. One whose payload has a
// representation no value of it uses -- a class reference, whose null
// is spare -- spends that on the empty case and is the same bytes as
// the payload. types.Sizeof already knows which is which, so this
// asks it rather than deciding again: a tag byte is there exactly
// when the whole is one byte wider than what it wraps.
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

// optionalTagField names the byte that says whether the value is
// there. Swift writes zero for the case that carries something and
// one for the case that does not, which is the order the cases are
// declared in.
const optionalTagField = "tag"

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
	// A thin metatype is nothing at run time: the type is known
	// statically, so the value carries no bits. It is passed the way
	// Void is passed, which is not at all -- an initializer's
	// metatype parameter is exactly this, and the convention says it
	// is there rather than that it costs a register.
	case *vil.MetatypeType:
		return f.Thin()
	}
	return false
}

// optionalOfType is the Optional a type is, if it is one.
func optionalOfType(t vil.Type) (*types.Optional, bool) {
	if !t.IsValid() || t.IsAddress() || t.Formal() == nil {
		return nil, false
	}
	o, ok := t.Formal().Underlying().(*types.Optional)
	return o, ok
}

// tupleImage is the struct a tuple's bytes look like.
//
// Named by position, which is how a tuple's elements are named
// anyway: `t.0` is the first. A labelled element keeps its label,
// because that is what a member expression will look for.
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
