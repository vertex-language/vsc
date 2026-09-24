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

// BasicKind identifies a built-in primitive or untyped literal type.
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

// OwnershipKind specifies the parameter passing convention (borrowing, consuming, or inout).
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

// Param describes a function or method parameter.
type Param struct {
	Name       string
	Label      string
	Type       Type
	Ownership  OwnershipKind
	Variadic   bool
	HasDefault bool
	// Autoclosure is a parameter declared @autoclosure: its type is a
	// function of no arguments, and what a call writes for it is the
	// expression the function evaluates, not the function.
	Autoclosure bool
	// Builder is the result builder the parameter is marked with --
	// `@Lines _ body: () -> [String]` -- whose calls a closure written
	// for it is made of. Nil for most parameters.
	Builder Type
	// Origin is the parameter this one was substituted from, where it
	// was: what was recorded about that one -- its default -- is this
	// one's too.
	Origin *Param
}

// BodyType returns the parameter's type within the function body (e.g. Array<T> for variadic T...).
func (p *Param) BodyType() Type {
	if p == nil {
		return nil
	}
	if p.Variadic {
		return &Array{Elem: p.Type}
	}
	return p.Type
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

// Signature represents a function type, including parameters, return type, and effects.
type Signature struct {
	TypeParams []*TypeParam
	Params     []*Param
	Results    Type
	Async      bool
	Throws     bool
	Thrown     Type // nil for untyped `throws`
	// Rethrows is `rethrows`: the function fails only by calling one of
	// its function arguments that does. Throws is set too.
	Rethrows bool
	// Exported is, for an initializer's signature, whether it was declared
	// public or open: another module may call it only then.
	Exported bool
	// Isolated is @MainActor: the function runs on the main thread, as
	// Swift's @MainActor functions run on the main actor. A synchronous
	// one is called from there or under `await`; an async one gets
	// itself there.
	Isolated bool
	// Failable is, for an initializer's signature, `init?`: a call makes
	// an optional, nil where the body returned nil.
	Failable bool
	// Convenience and Required are a class initializer's modifiers.
	// Inherited is a designated initializer a subclass has from its
	// superclass, declaring none of its own: the superclass's, run on the
	// subclass's instance once the subclass's own properties have their
	// defaults.
	Convenience bool
	Required    bool
	Inherited   bool
}

func (s *Signature) Underlying() Type { return s }

func (s *Signature) String() string {
	var sb strings.Builder
	if s.Isolated {
		sb.WriteString("@MainActor ")
	}
	sb.WriteString("(")
	for i, p := range s.Params {
		if i > 0 {
			sb.WriteString(", ")
		}
		if p.Ownership != DefaultOwnership {
			sb.WriteString(p.Ownership.String())
			sb.WriteString(" ")
		}
		if p.Type != nil {
			sb.WriteString(p.Type.String())
		}
		if p.Variadic {
			sb.WriteString("...")
		}
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

// Field represents a stored or computed property.
type Field struct {
	Name         string
	Type         Type
	IsConst      bool
	HasDefault   bool
	IsComputed   bool
	HasObservers bool
	// HasSetter is whether a computed property may be written to, which a
	// module interface states as `{ get set }` rather than `{ get }`.
	HasSetter bool
	// Isolated is @MainActor: the property is read and written on the
	// main thread only. See Signature.Isolated.
	Isolated bool
	// Exported is whether the property was declared public or open.
	// Another module reads or writes it only then -- though it lays out
	// every stored property, and an interface lists each.
	Exported bool
	// Origin is the property this one was substituted from, where it was;
	// see Method.Origin.
	Origin *Field
	// LazyStorage is, on a `lazy var`, the stored property that holds its
	// value once made -- Swift's `$__lazy_storage_$_name`, an optional --
	// whose getter makes it on the first read. LazyOf is, on that storage,
	// the property it is for: the memberwise initializer's label.
	LazyStorage string
	LazyOf      string
	// Ref is "weak" or "unowned" for a class-typed stored property that
	// does not keep its object alive: its word counts a weak reference.
	Ref string
	// Wrapper is, on a property a property wrapper gives -- `level` of
	// `@Clamped(0...10) var level = 5`, and its projection `$level` --
	// the stored property holding the wrapper, `_level`; the property is
	// the wrapper's wrappedValue, or with Projected its projectedValue.
	Wrapper   string
	Projected bool
}

// Method represents a method declaration or requirement.
type Method struct {
	Name       string
	Sig        *Signature
	IsStatic   bool
	IsMutating bool
	// IsConsuming is a `consuming func`: it takes self, which ends with it.
	IsConsuming bool
	// Exported is whether the method was declared public or open, which
	// is what a module interface writes and nothing else.
	Exported bool
	// Origin is the method this one was substituted from, where it was: a
	// protocol extension's method as a conforming type sees it, with Self
	// replaced. The declaration is found by the origin.
	Origin *Method
}

// Subscript is a subscript a type declares: `subscript(x: Int) -> T`.
// Its parameters carry no label unless one is written: `subscript(x: Int)`
// is used as `s[1]`, and `subscript(at i: Int)` as `s[at: 1]`.
type Subscript struct {
	// TypeParams are the subscript's own generic parameters,
	// `subscript<U>(...)`, which each use infers.
	TypeParams []*TypeParam
	Params     []*Param
	Result     Type
	IsStatic   bool
	// Settable is whether the subscript declares a setter.
	Settable bool
	// NonmutatingSet is a `nonmutating set`: it writes through self --
	// a buffer pointer's elements -- rather than to it, so a `let` may.
	NonmutatingSet bool
	Exported bool
}

// Requirement represents a protocol requirement.
type Requirement struct {
	Name    string
	Sig     *Signature // non-nil for method requirement
	Type    Type       // non-nil for property requirement
	IsVar   bool
	IsConst bool
	// IsStatic is a requirement of the type rather than of an instance;
	// IsMutating a method requirement's.
	IsStatic   bool
	IsMutating bool
}

// Named represents a nominal type alias or unresolved type name.
type Named struct {
	Name       string
	Pkg        string
	underlying Type
}

func NewNamed(name, pkg string, underlying Type) *Named {
	return &Named{Name: name, Pkg: pkg, underlying: underlying}
}

func (n *Named) SetUnderlying(t Type) { n.underlying = t }

// Aliased is the type a typealias names, or nil before it is resolved.
func (n *Named) Aliased() Type { return n.underlying }
func (n *Named) Underlying() Type {
	if n.underlying != nil {
		return n.underlying.Underlying()
	}
	return n
}
func (n *Named) String() string { return n.Name }

// Struct represents a nominal value type.
type Struct struct {
	Name         string
	TypeParams   []*TypeParam
	Fields       []*Field
	Methods      []*Method
	Inits        []*Signature
	Conformances []*Protocol
	Copyable     bool
	// Deinit is whether the struct -- a ~Copyable one -- declares a
	// deinit, which runs where a value of it ends.
	Deinit     bool
	Assoc      map[string]Type // associated type mappings
	In         Type            // enclosing type if nested
	Computed   []*Field        // computed properties
	Statics    []*Field        // static properties
	Subscripts []*Subscript
	// BodyInits is how many of Inits the struct's own body declares,
	// which come first. Only those take the memberwise initializer away:
	// one an extension declares is beside it, as in Swift.
	BodyInits int
}

// Memberwise returns the synthesized memberwise initializer signature for the struct,
// or nil if the struct's body declares its own initializers or has unresolvable fields.
func (s *Struct) Memberwise() *Signature {
	if s.BodyInits > 0 {
		return nil
	}
	sig := &Signature{TypeParams: s.TypeParams}
	for _, f := range s.Fields {
		if f == nil || f.Type == nil {
			return nil
		}
		label := f.Name
		if f.LazyOf != "" {
			label = f.LazyOf
		}
		sig.Params = append(sig.Params, &Param{
			Name:       label,
			Label:      label,
			Type:       f.Type,
			HasDefault: f.HasDefault,
		})
	}
	sig.Results = s
	return sig
}

// Minimum returns the minimum number of arguments required to call the function (excluding parameters with defaults).
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

// Class represents a nominal reference type.
type Class struct {
	Inits        []*Signature
	Name         string
	TypeParams   []*TypeParam
	Superclass   Type
	Fields       []*Field
	Methods      []*Method
	Conformances []*Protocol
	IsActor      bool
	Assoc        map[string]Type // associated type mappings
	In           Type            // enclosing type if nested
	Computed     []*Field        // computed properties
	Statics      []*Field        // static properties
	Subscripts   []*Subscript
	// Origin is the declared class this one is an instance's substituted
	// copy of -- Container for Container<Int>'s -- or nil for a declared one.
	Origin *Class
}

func (c *Class) Underlying() Type { return c }

// Declared is the class as declared: c, or what c was substituted from.
func (c *Class) Declared() *Class {
	if c.Origin != nil {
		return c.Origin
	}
	return c
}
func (c *Class) String() string { return c.Name }

// EnumCase represents an enum case declaration.
type EnumCase struct {
	Name           string
	AssociatedType Type
	// Label is a single associated value's label, as in `case bad(code: Int)`,
	// whose AssociatedType is the Int itself.
	Label string
	// Indirect is an `indirect case`, or a case of an `indirect enum`,
	// that carries a value: the value lives in a heap box and the enum
	// holds a reference to it, which is what lets an enum hold itself.
	Indirect  bool
	RawValue  string
	RawInt    int64
	HasRawInt bool
}

// Enum represents a nominal enumeration type.
type Enum struct {
	Name         string
	TypeParams   []*TypeParam
	RawType      Type
	Cases        []*EnumCase
	Methods      []*Method
	Conformances []*Protocol
	Copyable     bool
	Indirect     bool            // `indirect enum`: every case that carries a value is indirect
	Assoc        map[string]Type // associated type mappings
	In           Type            // enclosing type if nested
	Computed     []*Field        // computed properties
	Statics      []*Field        // static properties
	Subscripts   []*Subscript
	Inits        []*Signature // initializers the enum and its extensions declare
}

func (e *Enum) Underlying() Type { return e }
func (e *Enum) String() string   { return e.Name }

// Protocol represents a protocol definition.
type Protocol struct {
	Name         string
	Inherited    []*Protocol
	Requirements []*Requirement
	Associated   []*Associated
	Self         *TypeParam
	// Primary are the protocol's primary associated types, in order:
	// `protocol Source<Value>`, which `Source<Int>` gives in brackets.
	Primary []string
	// Subscripts are the subscripts the protocol requires:
	// Collection's `subscript(position: Index) -> Element { get }`.
	Subscripts []*Subscript
	// SameTypes are the protocol's own same-type requirements between
	// its associated types: Sequence's `Iterator.Element == Element` is
	// "Iterator.Element" to "Element".
	SameTypes map[string]string
	// What the protocol's extensions add: members every conforming type
	// has, written in terms of Self, which is the conforming type where
	// one is used.
	ExtMethods  []*Method
	ExtComputed []*Field
	ExtStatics  []*Field
}

// ExtensionMethod is the method named name an extension of p, or of a
// protocol p inherits, declares, with the protocol that declares it.
func (p *Protocol) ExtensionMethod(name string, static bool) (*Protocol, *Method) {
	return p.extensionMethod(name, static, map[*Protocol]bool{})
}

func (p *Protocol) extensionMethod(name string, static bool, seen map[*Protocol]bool) (*Protocol, *Method) {
	if p == nil || seen[p] {
		return nil, nil
	}
	seen[p] = true
	for _, m := range p.ExtMethods {
		if m.Name == name && m.IsStatic == static {
			return p, m
		}
	}
	for _, up := range p.Inherited {
		if q, m := up.extensionMethod(name, static, seen); m != nil {
			return q, m
		}
	}
	return nil, nil
}

// ExtensionMethods is every method named name the extensions of p and of
// the protocols it inherits declare: overloads, told apart by the call.
func (p *Protocol) ExtensionMethods(name string, static bool) []*Method {
	var out []*Method
	seen := map[*Protocol]bool{}
	var walk func(q *Protocol)
	walk = func(q *Protocol) {
		if q == nil || seen[q] {
			return
		}
		seen[q] = true
		for _, m := range q.ExtMethods {
			if m.Name == name && m.IsStatic == static {
				out = append(out, m)
			}
		}
		for _, up := range q.Inherited {
			walk(up)
		}
	}
	walk(p)
	return out
}

// ExtensionProperty is the computed property named name an extension of
// p, or of a protocol p inherits, declares.
func (p *Protocol) ExtensionProperty(name string, static bool) (*Protocol, *Field) {
	seen := map[*Protocol]bool{}
	var walk func(q *Protocol) (*Protocol, *Field)
	walk = func(q *Protocol) (*Protocol, *Field) {
		if q == nil || seen[q] {
			return nil, nil
		}
		seen[q] = true
		list := q.ExtComputed
		if static {
			list = q.ExtStatics
		}
		for _, f := range list {
			if f.Name == name {
				return q, f
			}
		}
		for _, up := range q.Inherited {
			if r, f := walk(up); f != nil {
				return r, f
			}
		}
		return nil, nil
	}
	return walk(p)
}

// OwnExtensionMethod is the method of the name p's own extensions declare
// -- not those of a protocol it refines -- that fits says true of, or nil.
func (p *Protocol) OwnExtensionMethod(name string, static bool, fits func(*Method) bool) *Method {
	for _, m := range p.ExtMethods {
		if m != nil && m.Name == name && m.IsStatic == static && m.Sig != nil && fits(m) {
			return m
		}
	}
	return nil
}

// IsExtensionMethod reports whether m is one p's extensions declare.
func (p *Protocol) IsExtensionMethod(m *Method) bool {
	if m == nil {
		return false
	}
	if m.Origin != nil {
		m = m.Origin
	}
	q, found := p.ExtensionMethod(m.Name, m.IsStatic)
	if found == m {
		return true
	}
	for _, x := range p.ExtensionMethods(m.Name, m.IsStatic) {
		if x == m {
			return true
		}
	}
	_ = q
	return false
}

// Associated represents an associated type requirement in a protocol.
type Associated struct {
	Name        string
	Constraints []*Protocol
}

// Dependent represents an associated type path (e.g. Self.Element or C.Element).
type Dependent struct {
	Base Type
	Name string
}

func (d *Dependent) Underlying() Type { return d }
func (d *Dependent) String() string {
	if d.Base == nil {
		return d.Name
	}
	return d.Base.String() + "." + d.Name
}

func (p *Protocol) Underlying() Type { return p }
func (p *Protocol) String() string   { return p.Name }

// ParameterizedProtocol is a protocol with its primary associated types
// given: `Collection<Int>`, a Collection whose Element is Int. It is what
// `some` and `any` constrain, and a generic parameter conforms to.
type ParameterizedProtocol struct {
	Protocol *Protocol
	Args     []Type
}

func (p *ParameterizedProtocol) Underlying() Type { return p }
func (p *ParameterizedProtocol) String() string {
	args := make([]string, len(p.Args))
	for i, a := range p.Args {
		args[i] = a.String()
	}
	return p.Protocol.Name + "<" + strings.Join(args, ", ") + ">"
}

// Same is what the arguments say of the protocol's primary associated
// types: Element is Int.
func (p *ParameterizedProtocol) Same() map[string]Type {
	out := map[string]Type{}
	for i, a := range p.Args {
		if i < len(p.Protocol.Primary) {
			out[p.Protocol.Primary[i]] = a
		}
	}
	return out
}

// Array is `[T]`. It is the same type as `Array<T>`, which is what
// the resolver reads both spellings into.
type Array struct {
	Elem Type
}

func (a *Array) Underlying() Type { return a }
func (a *Array) String() string   { return fmt.Sprintf("[%s]", a.Elem) }

// Set is `Set<Element>`: one reference to a hash table of distinct
// elements, the same representation a Dictionary has with no values.
type Set struct {
	Elem Type
}

func (s *Set) Underlying() Type { return s }
func (s *Set) String() string   { return fmt.Sprintf("Set<%s>", s.Elem) }

// Dictionary represents a dictionary type [Key: Value].
type Dictionary struct {
	Key   Type
	Value Type
}

func (d *Dictionary) Underlying() Type { return d }
func (d *Dictionary) String() string   { return fmt.Sprintf("[%s: %s]", d.Key, d.Value) }

// Pointer represents an unsafe pointer type (UnsafePointer, UnsafeMutablePointer, RawPointer, or OpaquePointer).
type Pointer struct {
	Elem    Type // pointee type; nil for raw or opaque pointers
	Mutable bool
	Opaque  bool
}

func (p *Pointer) Underlying() Type { return p }

func (p *Pointer) String() string {
	switch {
	case p.Opaque:
		return "OpaquePointer"
	case p.Elem == nil && p.Mutable:
		return "UnsafeMutableRawPointer"
	case p.Elem == nil:
		return "UnsafeRawPointer"
	case p.Mutable:
		return fmt.Sprintf("UnsafeMutablePointer<%s>", p.Elem)
	}
	return fmt.Sprintf("UnsafePointer<%s>", p.Elem)
}

// Dereferenceable reports whether the pointer can be dereferenced through `pointee`.
func (p *Pointer) Dereferenceable() bool { return p != nil && p.Elem != nil && !p.Opaque }

// Optional represents an optional type T?.
type Optional struct {
	Wrapped Type
	// Implicit is an optional written T!: Swift's implicitly unwrapped
	// optional, which is an Optional that is unwrapped wherever it is
	// used as the type it wraps. What a header says nothing about the
	// nullability of is imported this way. The flag lasts only as long as
	// the declaration's type; a value taken from one is a plain T?.
	Implicit bool
}

func (o *Optional) Underlying() Type { return o }
func (o *Optional) String() string   { return fmt.Sprintf("%s?", o.Wrapped) }

// Range represents a half-open (a..<b) or closed (a...b) range.
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

// Metatype represents a metatype T.Type.
type Metatype struct {
	Instance Type
}

func (m *Metatype) Underlying() Type { return m }
func (m *Metatype) String() string   { return fmt.Sprintf("%s.Type", m.Instance) }

// Existential represents an existential type (e.g. `any P` or `Any`).
type Existential struct {
	Protocols []*Protocol
	// Same is what primary associated types were given as: `any
	// Source<String>` has Value String.
	Same map[string]Type
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
	// Same is what primary associated types were given as: `some
	// Collection<Int>` has Element Int.
	Same map[string]Type
	// Concrete is, for a function's result, the type its body returns,
	// which is what `some P` stands for there.
	Concrete Type
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
	Bound       map[string]Type   // associated type equalities from where clauses
	Promised    map[string][]Type // associated type conformances from where clauses
	// Same is the one type the parameter is, inside an extension whose
	// where clause says so -- `extension Array where Element == String`
	// -- and nil everywhere else.
	Same Type
}

func (tp *TypeParam) Underlying() Type { return tp }
func (tp *TypeParam) String() string   { return tp.Name }

// GenericInstance represents a parameterized generic type (e.g. Stack<Int>).
type GenericInstance struct {
	Base     Type
	Args     []Type
	instance Type
}

// Underlying returns Base with its type parameters substituted with Args.
func (g *GenericInstance) Underlying() Type {
	if g.Base == nil {
		return g
	}
	if g.instance != nil {
		return g.instance
	}
	params := typeParamsOfType(g.Base)
	if len(params) == 0 || len(params) != len(g.Args) {
		return g.Base.Underlying()
	}
	subst := make(map[*TypeParam]Type, len(params))
	for i, p := range params {
		subst[p] = g.Args[i]
	}
	g.instance = Substitute(g.Base.Underlying(), subst)
	return g.instance
}

// InstanceSubst maps a generic instance's type parameters to its
// arguments -- Failure to NetError in Outcome<Int, NetError> -- or is
// nil for anything else.
func InstanceSubst(t Type) map[*TypeParam]Type {
	g, ok := t.(*GenericInstance)
	if !ok || g.Base == nil {
		return nil
	}
	params := typeParamsOfType(g.Base)
	if len(params) == 0 || len(params) != len(g.Args) {
		return nil
	}
	subst := make(map[*TypeParam]Type, len(params))
	for i, p := range params {
		subst[p] = g.Args[i]
	}
	return subst
}

// typeParamsOfType returns the type parameters declared on nominal type t.
func typeParamsOfType(t Type) []*TypeParam {
	switch n := t.(type) {
	case *Named:
		// A name this compiler could not resolve is its own
		// underlying type, so following it is a walk that does not
		// stop. There is nothing to find through one anyway.
		under := n.Underlying()
		if under == nil || under == t {
			return nil
		}
		return typeParamsOfType(under)
	case *Struct:
		return n.TypeParams
	case *Class:
		return n.TypeParams
	case *Enum:
		return n.TypeParams
	}
	return nil
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
