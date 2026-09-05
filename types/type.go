package types

import (
	"fmt"
	"strings"
)

// Type is what a spelling in the source resolved to. Underlying
// looks through the names: a typealias is another spelling of the
// type it names, and Underlying is where that ends.
type Type interface {
	// Underlying returns the underlying concrete type, stripping any Named wrapper.
	Underlying() Type
	// String returns a human-readable representation of the type.
	String() string
}

// BasicKind names one of the types the compiler knows without
// reading a declaration. The untyped kinds at the end are not types a
// program can write: they are what a literal is before the context
// around it says which type it takes.
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

// Basic is one of the builtin types. It has no members here — those
// live in a library this compiler has no reader for — so a member of
// one is unknown rather than absent.
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

// OwnershipKind is what a parameter does with the value it is given:
// borrow it for the call, consume it, or take a reference the callee
// writes through.
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

// Param is one parameter. Label is what a call writes and Name is
// what the body uses — `func move(to dest: Point)` has both, and a
// parameter written with one name has them the same.
type Param struct {
	Name      string
	Label     string
	Type      Type
	Ownership OwnershipKind
	Variadic  bool
	// HasDefault says the declaration gave this parameter a value, so
	// a call may leave it out. What the value is stays in the syntax:
	// nothing in this package holds an expression, and the default is
	// one.
	HasDefault bool
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

// Signature is a function type: what it takes, what it gives back,
// and the effects it has.
//
// Throwing is two facts, because Swift's is. A function that throws
// says so, and it may also name what it throws: `throws(E)` is a
// typed throw, plain `throws` names nothing and means any error, and
// `throws(Never)` is how the language spells a function that does not
// throw at all — so a thrown type of Never is not a throwing
// function, and the two fields keep that from being a puzzle.
type Signature struct {
	TypeParams []*TypeParam // the generic parameters, if the function has any
	Params     []*Param
	Results    Type
	Async      bool
	Throws     bool // the function may throw
	Thrown     Type // what it throws, or nil where `throws` named nothing
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
	if s.Throws {
		if s.Thrown == nil {
			sb.WriteString(" throws")
		} else {
			sb.WriteString(fmt.Sprintf(" throws(%s)", s.Thrown.String()))
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

// Tuple is a tuple type. Swift has no one-element tuple: `(Int)` is
// an Int in parentheses, and the parser says so before this.
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

// Field is a stored property. IsConst is the `let` of `let x: Int`,
// which is what makes an assignment to it a mistake.
type Field struct {
	Name    string
	Type    Type
	IsConst bool
	// HasDefault says the property was declared with an initial
	// value, which is what lets the memberwise initializer leave its
	// parameter out.
	HasDefault bool
}

// Method is a function declared in a type, or promised by a
// protocol. Static and instance methods are not yet told apart.
type Method struct {
	Name string
	Sig  *Signature
}

// Requirement is what a protocol promises: a method, with a
// signature, or a property, with a type.
type Requirement struct {
	Name    string
	Sig     *Signature // non-nil for method requirement
	Type    Type       // non-nil for property requirement
	IsVar   bool
	IsConst bool
}

// Named is a name standing for a type. With an underlying type it is
// a typealias, which Identical looks through; with none it is a name
// this compiler could not resolve, and stays opaque rather than
// pretending to be something.
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

// Struct is a value type: its instances are copied, and its fields
// are laid out inline. Copyable is false for one declared
// `~Copyable`.
type Struct struct {
	Name         string
	TypeParams   []*TypeParam
	Fields       []*Field
	Methods      []*Method
	Inits        []*Signature
	Conformances []*Protocol
	Copyable     bool
}

// Memberwise is the initializer a struct gets for free: one parameter
// per stored property, in declaration order, labelled with the
// property's name.
//
// Swift synthesizes it only where the type declares no initializer of
// its own, which is the rule this follows — a struct with an `init`
// has said what making one means, and the free one would be a second
// answer to the same question. It is nil for such a struct, and for
// one whose fields did not resolve.
//
// A `let` property with an initial value is still a parameter here,
// which is Swift's rule and not an oversight: the memberwise
// initializer takes every stored property, and it is the *default*
// that a property with a value gets, not its exclusion.
func (s *Struct) Memberwise() *Signature {
	if len(s.Inits) > 0 {
		return nil
	}
	// The struct's type parameters are the initializer's, so that
	// `Wrapper(value: 3)` infers T from the argument the way any
	// other generic call does.
	sig := &Signature{TypeParams: s.TypeParams}
	for _, f := range s.Fields {
		if f == nil || f.Type == nil {
			return nil
		}
		sig.Params = append(sig.Params, &Param{
			Name:  f.Name,
			Label: f.Name,
			Type:  f.Type,
			// A property with an initial value gives its parameter
			// one, which is what makes `Inner()` legal for a struct
			// whose properties all have defaults.
			HasDefault: f.HasDefault,
		})
	}
	sig.Results = s
	return sig
}

// Minimum is how many arguments a call must supply: the parameters
// without a default. A call may give more, up to all of them.
func (s *Signature) Minimum() int {
	n := 0
	for _, p := range s.Params {
		if p != nil && !p.HasDefault {
			n++
		}
	}
	return n
}

func (s *Struct) Underlying() Type { return s }
func (s *Struct) String() string   { return s.Name }

// Class is a reference type: its instances are one object with many
// references to it. An actor is a Class with IsActor set, because
// that is what an actor is, plus a rule about who may touch it.
type Class struct {
	// Inits are the initializers the class declares. A class gets no
	// memberwise initializer — Swift gives that to structs alone,
	// because a class has inheritance and initialization has to
	// account for a superclass.
	Inits []*Signature

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

// EnumCase is one case. AssociatedType is what it carries — a single
// type, or a Tuple where it carries several — and RawValue is the
// other kind of case, the one numbered or named by its declaration.
type EnumCase struct {
	Name           string
	AssociatedType Type
	RawValue       string
}

// Enum is a type that is one of its cases. RawType is set for one
// declared with a raw value type, which is the form that has no
// associated values.
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

// Protocol is a set of requirements a type may declare it meets.
// Conformance is nominal: see ConformsTo.
type Protocol struct {
	Name         string
	Inherited    []*Protocol
	Requirements []*Requirement
}

func (p *Protocol) Underlying() Type { return p }
func (p *Protocol) String() string   { return p.Name }

// Array is `[T]`. It is the same type as `Array<T>`, which is what
// the resolver reads both spellings into.
type Array struct {
	Elem Type
}

func (a *Array) Underlying() Type { return a }
func (a *Array) String() string   { return fmt.Sprintf("[%s]", a.Elem) }

// Dictionary is `[Key: Value]`, and `Dictionary<Key, Value>` with
// it.
type Dictionary struct {
	Key   Type
	Value Type
}

func (d *Dictionary) Underlying() Type { return d }
func (d *Dictionary) String() string   { return fmt.Sprintf("[%s: %s]", d.Key, d.Value) }

// Optional is `T?`: the wrapped value, or none. What it costs is in
// layout.go, and it is not always a byte more than T.
type Optional struct {
	Wrapped Type
}

func (o *Optional) Underlying() Type { return o }
func (o *Optional) String() string   { return fmt.Sprintf("%s?", o.Wrapped) }

// Range is `a..<b` or `a...b`: two bounds of the same type, and
// whether the upper one is included.
//
// Swift has two types here, Range and ClosedRange, because they
// differ in more than a flag — a closed range over the whole of Int
// has no representable "one past the end" and its iterator has to
// carry a finished bit. They are one type with a flag here because
// nothing yet distinguishes them beyond how a for-in counts, and
// splitting them before there is a second difference would be two
// names for one idea.
type Range struct {
	Element Type
	Closed  bool
}

func (r *Range) Underlying() Type { return r }
func (r *Range) String() string {
	if r.Closed {
		return fmt.Sprintf("ClosedRange<%s>", r.Element)
	}
	return fmt.Sprintf("Range<%s>", r.Element)
}

// Metatype is `T.Type`: the type itself as a value. A type's name in
// expression position denotes one, which is what makes `Int.self` a
// value and `Box(v: 3)` a call to an initializer.
type Metatype struct {
	Instance Type
}

func (m *Metatype) Underlying() Type { return m }
func (m *Metatype) String() string   { return fmt.Sprintf("%s.Type", m.Instance) }

// Existential is `any P`: a value of some type conforming to the
// protocols, in a box, with the type erased. An empty one is `Any`.
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

// Opaque is `some P`: one type conforming to the constraints, the
// same one every time, which the caller is not told the name of.
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

// TypeParam is the `T` of `func f<T>(…)`. Identity is the pointer:
// two parameters with the same name in different declarations are
// different types, and Substitute is keyed on that.
type TypeParam struct {
	Name        string
	Constraints []Type
}

func (tp *TypeParam) Underlying() Type { return tp }
func (tp *TypeParam) String() string   { return tp.Name }

// GenericInstance is a generic type with its arguments given —
// `Stack<Int>`. A member of one is the member of Base with the
// parameters substituted.
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
