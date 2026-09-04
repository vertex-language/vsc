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

	case *types.Struct:
		return m.nominal(t.Name, Struct)
	case *types.Class:
		return m.nominal(t.Name, Class)
	case *types.Enum:
		return m.nominal(t.Name, Enum)

	case *types.Signature:
		return m.function(t)

	case *types.Protocol, *types.Existential:
		return fail(ErrUnsupported, "a protocol or existential type")
	case *types.TypeParam:
		return fail(ErrUnsupported, "a generic parameter")
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
	key := "nominal:" + m.moduleName + "." + name
	if i, ok := m.lookup(key); ok {
		m.substitution(i)
		return nil
	}
	if err := m.module(m.moduleName); err != nil {
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
	for i, e := range t.Elements {
		if e.Name != "" {
			if err := m.identifier(e.Name); err != nil {
				return err
			}
		}
		if err := m.typ(e.Type); err != nil {
			return err
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
