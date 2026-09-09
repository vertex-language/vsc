package mangle

import (
	"github.com/vertex-language/vsc/types"
)

// standard is the table of types the scheme gives a letter of their
// own. They are never numbered: a letter is already shorter than any
// back-reference could be.
var standard = map[types.BasicKind]byte{
	types.Int:       'i',
	types.UInt:      'u',
	types.Bool:      'b',
	types.Float:     'f',
	types.Double:    'd',
	types.String:    'S',
	types.Character: 'J',
}

// stdlib is the table of types that live in the standard library and
// have no letter, so they are written out against its one-character
// module name.
var stdlib = map[types.BasicKind]struct {
	name string
	kind NominalKind
}{
	types.Int8:   {"Int8", Struct},
	types.Int16:  {"Int16", Struct},
	types.Int32:  {"Int32", Struct},
	types.Int64:  {"Int64", Struct},
	types.UInt8:  {"UInt8", Struct},
	types.UInt16: {"UInt16", Struct},
	types.UInt32: {"UInt32", Struct},
	types.UInt64: {"UInt64", Struct},
	types.Never:  {"Never", Enum},
}

// typ writes one type.
func (m *mangler) typ(t types.Type) error {
	if t == nil {
		// A function that returns nothing returns the empty tuple, and
		// the empty tuple is `y`.
		m.writeByte('y')
		return nil
	}

	switch t := t.(type) {
	case *types.Basic:
		return m.basic(t)

	case *types.Named:
		return m.named(t)

	case *types.Tuple:
		return m.tuple(t)

	case *types.Optional:
		if err := m.typ(t.Wrapped); err != nil {
			return err
		}
		m.write("Sg")
		return nil

	case *types.Array:
		m.write("Sa")
		m.writeByte('y')
		if err := m.typ(t.Elem); err != nil {
			return err
		}
		m.writeByte('G')
		return nil

	// The unsafe pointers. Four have a standard substitution of their
	// own -- Swift spends two letters on each because a C-facing
	// signature is made of them -- and OpaquePointer does not, being
	// an ordinary struct in the standard library.
	//
	//	SP  UnsafePointer            SV  UnsafeRawPointer
	//	Sp  UnsafeMutablePointer     Sv  UnsafeMutableRawPointer
	//
	// The typed ones take their element the way Array takes its own:
	// the nominal, `y`, the argument, `G`.
	case *types.Pointer:
		switch {
		case t.Opaque:
			m.write("s13OpaquePointerV")
			return nil
		case t.Elem == nil && t.Mutable:
			m.write("Sv")
			return nil
		case t.Elem == nil:
			m.write("SV")
			return nil
		case t.Mutable:
			m.write("Sp")
		default:
			m.write("SP")
		}
		m.writeByte('y')
		if err := m.typ(t.Elem); err != nil {
			return err
		}
		m.writeByte('G')
		return nil

	// A generic type with its arguments given. Swift writes the
	// nominal, then `y`, then the arguments, then `G` -- the same
	// shape Array uses above, which is that spelling with the
	// nominal already known.
	case *types.GenericInstance:
		return m.boundGeneric(t)

	case *types.Struct, *types.Class, *types.Enum:
		return m.nominalType(t)

	case *types.Signature:
		return m.function(t)

	// `any P` -- a value whose type is not known, carrying the
	// protocols it satisfies. Swift writes the protocol list and then
	// `_p`: `AA1P_p` for one, and the list in order for more.
	case *types.Protocol:
		if err := m.protocolName(m.moduleFor(t), t.Name); err != nil {
			return err
		}
		m.write("_p")
		return nil

	case *types.Existential:
		if len(t.Protocols) == 0 {
			// `Any`, which constrains nothing: the empty protocol
			// list and the marker that makes it an existential.
			// swiftc writes `yp`, which is what the parameter list of
			// `print` is made of.
			m.write("yp")
			return nil
		}
		for _, p := range t.Protocols {
			if p == nil {
				return fail(ErrUnsupported, "an existential with no protocol")
			}
			if err := m.protocolName(m.moduleFor(p), p.Name); err != nil {
				return err
			}
		}
		m.write("_p")
		return nil
	case *types.TypeParam:
		// The first parameter at depth zero is `x`; every later one
		// is `q` and a mangled index, so the second is `q_` and the
		// third `q0_`. swiftc writes `x_q_q0_t` for a function of
		// three of them.
		i := m.paramIndex(t)
		if i < 0 {
			return fail(ErrUnsupported, "a generic parameter of no signature in scope")
		}
		if i == 0 {
			m.writeByte('x')
			return nil
		}
		m.writeByte('q')
		m.index(i - 1)
		return nil
	}
	return fail(ErrType, t.String())
}

func (m *mangler) basic(b *types.Basic) error {
	if s, ok := standard[b.Kind()]; ok {
		m.std(s)
		return nil
	}
	if s, ok := stdlib[b.Kind()]; ok {
		return m.stdlibNominal(s.name, s.kind)
	}
	switch b.Kind() {
	case types.Void:
		// A Void written as a type is the empty tuple.
		m.write("yt")
		return nil
	}
	return fail(ErrType, b.Name())
}

// named reaches through a name to what it names. A typealias is not
// part of a symbol: two declarations that differ only in which spelling
// of a type they used are the same declaration.
func (m *mangler) named(n *types.Named) error {
	switch u := n.Underlying().(type) {
	case *types.Struct:
		return m.nominal(n.Name, Struct)
	case *types.Class:
		return m.nominal(n.Name, Class)
	case *types.Enum:
		return m.nominal(n.Name, Enum)
	default:
		return m.typ(u)
	}
}

// nominal writes a named type declared in the module being compiled,
// or a back-reference to one already written.
func (m *mangler) nominal(name string, kind NominalKind) error {
	return m.nominalIn(m.moduleName, name, kind)
}

// moduleFor is where a type was declared: what the caller says, or
// the module being compiled where it says nothing.
func (m *mangler) moduleFor(t types.Type) string {
	if m.moduleOf != nil {
		if name := m.moduleOf(t); name != "" {
			return name
		}
	}
	return m.moduleName
}

// nominalType writes a named type, with whatever it is declared
// inside written before it.
//
// A nested type's symbol says the chain outermost first, each name
// followed by its own kind letter: swiftc writes `Outer.Inner` as
// `AA5OuterV5InnerV`, where the module is on the outermost and the
// inner one has none of its own. So the enclosing type is mangled
// first and that is what puts the module in.
func (m *mangler) nominalType(t types.Type) error {
	name, kind, ok := nominalOf(t)
	if !ok {
		return fail(ErrType, t.String())
	}
	module := m.moduleFor(t)
	key := "nominal:" + module + "." + chainOf(t)
	if i, ok := m.lookup(key); ok {
		m.substitution(i)
		return nil
	}
	if in := enclosing(t); in != nil {
		if err := m.nominalType(in); err != nil {
			return err
		}
	} else if err := m.module(module); err != nil {
		return err
	}
	if err := m.identifier(name); err != nil {
		return err
	}
	m.writeByte(byte(kind))
	m.remember(key)
	return nil
}

// nominalOf is a nominal type's name and the letter that says which
// kind of nominal it is.
func nominalOf(t types.Type) (string, NominalKind, bool) {
	switch n := t.(type) {
	case *types.Struct:
		return n.Name, Struct, true
	case *types.Class:
		return n.Name, Class, true
	case *types.Enum:
		return n.Name, Enum, true
	}
	return "", 0, false
}

// enclosing is the type a nominal one is declared inside, or nil.
func enclosing(t types.Type) types.Type {
	switch n := t.(type) {
	case *types.Struct:
		return n.In
	case *types.Class:
		return n.In
	case *types.Enum:
		return n.In
	}
	return nil
}

// chainOf is a nested type's dotted name, outermost first, which is
// what a substitution of it stands for.
func chainOf(t types.Type) string {
	name, _, ok := nominalOf(t)
	if !ok {
		return ""
	}
	if in := enclosing(t); in != nil {
		return chainOf(in) + "." + name
	}
	return name
}

// nominalIn writes a named type declared in a given module.
func (m *mangler) nominalIn(module, name string, kind NominalKind) error {
	key := "nominal:" + module + "." + name
	if i, ok := m.lookup(key); ok {
		m.substitution(i)
		return nil
	}
	if err := m.module(module); err != nil {
		return err
	}
	if err := m.identifier(name); err != nil {
		return err
	}
	m.writeByte(byte(kind))
	m.remember(key)
	return nil
}

// stdlibNominal writes a named type from the standard library, which
// differs only in that its module is the single letter s.
func (m *mangler) stdlibNominal(name string, kind NominalKind) error {
	key := "nominal:Swift." + name
	if i, ok := m.lookup(key); ok {
		m.substitution(i)
		return nil
	}
	m.writeByte('s')
	if err := m.identifier(name); err != nil {
		return err
	}
	m.writeByte(byte(kind))
	m.remember(key)
	return nil
}

func (m *mangler) tuple(t *types.Tuple) error {
	if len(t.Elements) == 0 {
		m.writeByte('y')
		return nil
	}
	// The label follows its element's type rather than preceding it:
	// swiftc writes `s5Int32V2lo_AD2hit` for `(lo: Int32, hi: Int32)`,
	// where each type is followed by the name it was given. Writing
	// the label first spells a different symbol -- and one that reads
	// correctly when demangled, which is how it got there.
	for i, e := range t.Elements {
		if err := m.typ(e.Type); err != nil {
			return err
		}
		if e.Name != "" {
			if err := m.identifier(e.Name); err != nil {
				return err
			}
		}
		if i == 0 {
			m.writeByte('_')
		}
	}
	m.writeByte('t')
	return nil
}

// function writes a function type: its result, then its parameters,
// then what kind of function it is.
func (m *mangler) function(sig *types.Signature) error {
	if len(sig.TypeParams) != 0 {
		return fail(ErrUnsupported, "a generic function type")
	}
	if err := m.typ(sig.Results); err != nil {
		return err
	}
	if err := m.params(sig.Params, false); err != nil {
		return err
	}
	if sig.Throws {
		m.writeByte('K')
	}
	// A function written as a parameter type does not escape unless it
	// says so, and the mangling records which it is.
	m.write("XE")
	return nil
}

// boundGeneric writes a generic type with its arguments: the nominal,
// then the arguments between `y` and `G`.
func (m *mangler) boundGeneric(t *types.GenericInstance) error {
	if t.Base == nil {
		return fail(ErrUnsupported, "a generic type with no base")
	}
	if err := m.typ(t.Base); err != nil {
		return err
	}
	m.writeByte('y')
	for _, a := range t.Args {
		if err := m.typ(a); err != nil {
			return err
		}
	}
	m.writeByte('G')
	return nil
}

// protocolName writes a protocol in an existential: the module and
// the name, and no kind letter.
//
// Every other nominal ends in one -- V for a struct, C for a class --
// and a protocol does not, because the `_p` that closes the list is
// what says these were protocols. swiftc writes `AA1P_p`, not
// `AA1PP_p`.
func (m *mangler) protocolName(module, name string) error {
	// A protocol is its module and its name, with no kind letter after
	// it -- the one nominal spelled that way -- and it is not
	// remembered as a thing of its own. A second `any P` in the same
	// symbol is the module and the name again, each by
	// back-reference, which merge into one run: swiftc writes
	// `AA1P_p` for the first and `AaE_p` for the second, where `AaE`
	// is the module's entry and the name's.
	if err := m.module(module); err != nil {
		return err
	}
	return m.identifier(name)
}
