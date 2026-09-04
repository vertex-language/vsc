// Package types models the semantic type system of the Vertex/Swift language.
package types

import (
	"fmt"
	"strings"
)

// Type represents a semantic type.
type Type interface {
	// Underlying returns the underlying concrete type, stripping any Named wrapper.
	Underlying() Type
	// String returns a human-readable representation of the type.
	String() string
}

// BasicKind represents the category of a primitive type.
type BasicKind int

const (
	Invalid BasicKind = iota
	Bool
	Int
	Int8
	Int16
	Int32
	Int64
	UInt
	UInt8
	UInt16
	UInt32
	UInt64
	Float
	Double
	String
	Character
	Void
	Never

	// Untyped types for literal type deduction before resolution.
	UntypedBool
	UntypedInt
	UntypedFloat
	UntypedString
	UntypedNil
)

// BasicInfo flags describing basic type properties.
type BasicInfo int

const (
	IsBoolean BasicInfo = 1 << iota
	IsInteger
	IsUnsigned
	IsFloat
	IsString
	IsUntyped
	IsNumeric = IsInteger | IsFloat
)

// Basic represents a built-in primitive type.
type Basic struct {
	kind BasicKind
	info BasicInfo
	name string
}

func (b *Basic) Kind() BasicKind  { return b.kind }
func (b *Basic) Info() BasicInfo  { return b.info }
func (b *Basic) Name() string     { return b.name }
func (b *Basic) Underlying() Type { return b }
func (b *Basic) String() string   { return b.name }

// OwnershipKind represents ownership semantics for parameters and values.
type OwnershipKind int

const (
	DefaultOwnership OwnershipKind = iota
	Borrowing
	Consuming
	InOut
)

func (o OwnershipKind) String() string {
	switch o {
	case Borrowing:
		return "borrowing"
	case Consuming:
		return "consuming"
	case InOut:
		return "inout"
	default:
		return ""
	}
}

// Param represents a parameter in a function signature.
type Param struct {
	Name      string
	Label     string
	Type      Type
	Ownership OwnershipKind
	Variadic  bool
}

func (p *Param) String() string {
	var sb strings.Builder
	if p.Ownership != DefaultOwnership {
		sb.WriteString(p.Ownership.String())
		sb.WriteString(" ")
	}
	if p.Label != "" && p.Label != "_" {
		sb.WriteString(p.Label)
		sb.WriteString(" ")
	} else if p.Label == "_" {
		sb.WriteString("_ ")
	}
	if p.Name != "" {
		sb.WriteString(p.Name)
		sb.WriteString(": ")
	}
	if p.Type != nil {
		sb.WriteString(p.Type.String())
	}
	if p.Variadic {
		sb.WriteString("...")
	}
	return sb.String()
}

// Signature represents a function or closure type.
type Signature struct {
	Params  []*Param
	Results Type
	Async   bool
	Throws  Type // nil if non-throwing; Typ[Never] / Error type or specific type if typed throws
}

func (s *Signature) Underlying() Type { return s }
func (s *Signature) String() string {
	var sb strings.Builder
	sb.WriteString("(")
	for i, p := range s.Params {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(p.String())
	}
	sb.WriteString(")")
	if s.Async {
		sb.WriteString(" async")
	}
	if s.Throws != nil {
		if s.Throws == Typ[Never] {
			sb.WriteString(" throws")
		} else {
			sb.WriteString(fmt.Sprintf(" throws(%s)", s.Throws.String()))
		}
	}
	sb.WriteString(" -> ")
	if s.Results != nil {
		sb.WriteString(s.Results.String())
	} else {
		sb.WriteString("Void")
	}
	return sb.String()
}

// TupleElement is one element in a tuple type.
type TupleElement struct {
	Name string
	Type Type
}

// Tuple represents a tuple type.
type Tuple struct {
	Elements []*TupleElement
}

func (t *Tuple) Underlying() Type { return t }
func (t *Tuple) String() string {
	var sb strings.Builder
	sb.WriteString("(")
	for i, elem := range t.Elements {
		if i > 0 {
			sb.WriteString(", ")
		}
		if elem.Name != "" {
			sb.WriteString(elem.Name)
			sb.WriteString(": ")
		}
		if elem.Type != nil {
			sb.WriteString(elem.Type.String())
		}
	}
	sb.WriteString(")")
	return sb.String()
}

// Field represents a property or member variable in a struct or class.
type Field struct {
	Name    string
	Type    Type
	IsConst bool
}

// Method represents a member method declared in a nominal type or protocol requirement.
type Method struct {
	Name string
	Sig  *Signature
}

// Requirement represents a requirement in a protocol (method or property).
type Requirement struct {
	Name    string
	Sig     *Signature // non-nil for method requirement
	Type    Type       // non-nil for property requirement
	IsVar   bool
	IsConst bool
}

// Named represents a named nominal type or type alias.
type Named struct {
	Name       string
	Pkg        string
	underlying Type
}

func NewNamed(name, pkg string, underlying Type) *Named {
	return &Named{Name: name, Pkg: pkg, underlying: underlying}
}

func (n *Named) SetUnderlying(t Type) { n.underlying = t }
func (n *Named) Underlying() Type {
	if n.underlying != nil {
		return n.underlying.Underlying()
	}
	return n
}
func (n *Named) String() string { return n.Name }

// Struct represents a nominal value type with inline layout.
type Struct struct {
	Name         string
	TypeParams   []*TypeParam
	Fields       []*Field
	Methods      []*Method
	Conformances []*Protocol
	Copyable     bool
}

func (s *Struct) Underlying() Type { return s }
func (s *Struct) String() string   { return s.Name }

// Class represents a nominal heap-allocated reference type.
type Class struct {
	Name         string
	TypeParams   []*TypeParam
	Superclass   Type
	Fields       []*Field
	Methods      []*Method
	Conformances []*Protocol
	IsActor      bool
}

func (c *Class) Underlying() Type { return c }
func (c *Class) String() string   { return c.Name }

// EnumCase represents one case in an enum.
type EnumCase struct {
	Name           string
	AssociatedType Type
	RawValue       string
}

// Enum represents a sum / algebraic data type.
type Enum struct {
	Name         string
	TypeParams   []*TypeParam
	RawType      Type
	Cases        []*EnumCase
	Methods      []*Method
	Conformances []*Protocol
	Copyable     bool
}

func (e *Enum) Underlying() Type { return e }
func (e *Enum) String() string   { return e.Name }

// Protocol represents an interface / constraint contract.
type Protocol struct {
	Name         string
	Inherited    []*Protocol
	Requirements []*Requirement
}

func (p *Protocol) Underlying() Type { return p }
func (p *Protocol) String() string   { return p.Name }

// Array represents an array type [T].
type Array struct {
	Elem Type
}

func (a *Array) Underlying() Type { return a }
func (a *Array) String() string   { return fmt.Sprintf("[%s]", a.Elem) }

// Dictionary represents a dictionary type [Key: Value].
type Dictionary struct {
	Key   Type
	Value Type
}

func (d *Dictionary) Underlying() Type { return d }
func (d *Dictionary) String() string   { return fmt.Sprintf("[%s: %s]", d.Key, d.Value) }

// Optional represents a nullable value type T?.
type Optional struct {
	Wrapped Type
}

func (o *Optional) Underlying() Type { return o }
func (o *Optional) String() string   { return fmt.Sprintf("%s?", o.Wrapped) }

// Metatype represents the metatype of a type (T.Type).
type Metatype struct {
	Instance Type
}

func (m *Metatype) Underlying() Type { return m }
func (m *Metatype) String() string   { return fmt.Sprintf("%s.Type", m.Instance) }

// Existential represents an existential container `any P`.
type Existential struct {
	Protocols []*Protocol
}

func (e *Existential) Underlying() Type { return e }
func (e *Existential) String() string {
	if len(e.Protocols) == 0 {
		return "Any"
	}
	names := make([]string, len(e.Protocols))
	for i, p := range e.Protocols {
		names[i] = p.Name
	}
	return "any " + strings.Join(names, " & ")
}

// Opaque represents an opaque return type `some P`.
type Opaque struct {
	Base        Type
	Constraints []*Protocol
}

func (o *Opaque) Underlying() Type { return o }
func (o *Opaque) String() string {
	if len(o.Constraints) == 0 {
		return "some Any"
	}
	names := make([]string, len(o.Constraints))
	for i, p := range o.Constraints {
		names[i] = p.Name
	}
	return "some " + strings.Join(names, " & ")
}

// TypeParam represents a generic type parameter.
type TypeParam struct {
	Name        string
	Constraints []Type
}

func (tp *TypeParam) Underlying() Type { return tp }
func (tp *TypeParam) String() string   { return tp.Name }

// GenericInstance represents an instantiated generic type (e.g. Array<Int>, Stack<T>).
type GenericInstance struct {
	Base Type
	Args []Type
}

func (g *GenericInstance) Underlying() Type {
	if g.Base != nil {
		return g.Base.Underlying()
	}
	return g
}

func (g *GenericInstance) String() string {
	var sb strings.Builder
	if g.Base != nil {
		sb.WriteString(g.Base.String())
	}
	sb.WriteString("<")
	for i, arg := range g.Args {
		if i > 0 {
			sb.WriteString(", ")
		}
		if arg != nil {
			sb.WriteString(arg.String())
		} else {
			sb.WriteString("?")
		}
	}
	sb.WriteString(">")
	return sb.String()
}
