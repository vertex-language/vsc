package analyzer

import (
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/token"
	"github.com/vertex-language/vsc/types"
)

// Info holds the results of semantic analysis for a parsed unit or package.
type Info struct {
	// Types maps each evaluated expression to its resolved semantic type.
	Types map[ast.Expr]types.Type

	// Defs maps each identifier that defines a symbol to that symbol.
	Defs map[*ast.Ident]Symbol

	// Uses maps each identifier that refers to a symbol to that symbol.
	Uses map[*ast.Ident]Symbol

	// Scopes maps AST nodes (File, CodeBlock, StructDecl, etc.) to their lexical scope.
	Scopes map[ast.Node]*Scope

	// Folded maps each flat SequenceExpr to its precedence-folded expression tree.
	Folded map[*ast.SequenceExpr]ast.Expr

	// Operators maps operator expressions to resolved symbols.
	Operators map[ast.Expr]Symbol

	// PatternTypes are the types a catch pattern tests the error against:
	// T of `is T` and `let e as T`, and the enum of `E.case`.
	PatternTypes map[ast.Pattern]types.Type

	// ArrayRepeats are the calls `Array(repeating:count:)`.
	ArrayRepeats map[*ast.CallExpr]bool

	// CoreCalls are the calls that mean a function of the core's --
	// Dictionary(grouping:by:) is _dictionaryGrouping -- each as the call
	// of it the checker checked.
	CoreCalls map[*ast.CallExpr]*ast.CallExpr

	// OperatorSpecs are the operators named as values that mean a generic
	// function, each with what its parameters stand for there.
	OperatorSpecs map[ast.Expr]Specialization

	// OperatorCalls are the operators that mean a generic function --
	// Swift's `==` on tuples -- each as the specialized call it is.
	OperatorCalls map[ast.Expr]*ast.CallExpr

	// PatternMatches are the expression patterns matched through a `~=`
	// the program declares.
	PatternMatches map[*ast.ExprPattern]*FuncSymbol

	// CallSites are the calls that leave a parameter to its default.
	CallSites map[*ast.CallExpr]CallSite

	// AsyncTopLevel is whether main.swift's top-level code awaits, which
	// makes it the body of an async main.
	AsyncTopLevel bool

	// ArraySequences are the calls `Array(s)` of a sequence that is not
	// an array -- a String's Characters, a Sequence of the program's own
	// -- each an array of the elements an iteration of s gives.
	ArraySequences map[*ast.CallExpr]*Iteration

	// Autoclosures are the arguments passed for an @autoclosure
	// parameter, each the body of the function it is passed as.
	Autoclosures map[ast.Expr]*types.Signature

	// Captures are the names a closure's capture list binds: `[x]` and
	// `[y = x * 100]` each a constant of the closure's, whose value is
	// taken when the closure is made.
	Captures map[*ast.CaptureItem]*Capture

	// LiteralInits are the literals written where a type that is
	// expressible by them is wanted -- `let m: Money = 12`, a Character's
	// "a" -- each made by that type's literal initializer.
	LiteralInits map[ast.Expr]*LiteralInit

	// CoreTypes are the types core.swift declares: Task, whose members are
	// calls into the runtime rather than code of their own.
	CoreTypes map[types.Type]bool

	// Values maps literal and string segment nodes to decoded values.
	Values map[ast.Node]Value

	// Methods maps member expressions to resolved method references.
	Methods map[*ast.MemberExpr]*MethodRef

	// Extensions maps extension declarations to extended types.
	Extensions map[*ast.ExtensionDecl]types.Type
	// Builtins are the members extensions give the built-in types, by
	// BuiltinKey; see builtin.go.
	Builtins map[string]*BuiltinMembers
	// ImplicitSelf is, for a name used alone inside an extension of a
	// built-in type that means a member of self, the `self.name` it means.
	ImplicitSelf map[ast.Expr]ast.Expr

	// Receivers maps receiver methods to their receiver types.
	Receivers map[*ast.FuncDecl]types.Type

	// FieldDefaults maps stored properties to default value expressions.
	FieldDefaults map[*types.Field]ast.Expr

	// Specializations maps generic call expressions to type specializations.
	Specializations map[*ast.CallExpr]Specialization

	// Defaults maps parameters to default value expressions.
	Defaults map[*types.Param]ast.Expr

	// RawInits are the calls `E(rawValue: x)` on an enum with a raw type,
	// which are an E? -- nil where no case has that raw value.
	RawInits map[*ast.CallExpr]*types.Enum

	// Inits are the initializer each call of a type chose, as the type
	// declares it: by labels, where one is left out for its default, and
	// not only by how many arguments are written.
	Inits map[*ast.CallExpr]*types.Signature

	// OperatorMethods are the operators that mean a static method of a
	// type: `a + b` where a's type declares `static func +`. Operators
	// holds the ones that mean a function instead.
	OperatorMethods map[ast.Expr]*MethodRef

	// conditions are what a constrained extension asks of its type's
	// parameters before a member it adds may be used, by the member.
	conditions map[any][]memberCondition

	// ImplicitMethods are the leading-dot calls of a static method:
	// `.seconds(1)` where a Span is wanted, for `Span.seconds(1)`.
	ImplicitMethods map[*ast.ImplicitMemberExpr]*MethodRef

	// DerivedOperators are the operators that mean another one, the way
	// Equatable's `!=` means `==` negated.
	DerivedOperators map[ast.Expr]*DerivedOperator

	// EmptyCollections are the calls that make an empty collection of a
	// type they name: `Set<Int>()`, `[String: Int]()`.
	EmptyCollections map[*ast.CallExpr]types.Type

	// ArrayCopies are the calls `Array(xs)` of an array: a copy of it.
	ArrayCopies map[*ast.CallExpr]types.Type

	// OptionalSomes are the calls that wrap a value -- `.some(x)`,
	// `Optional(x)`, `Optional<T>(x)`, `Optional.some(x)` -- and
	// OptionalNones the expressions that are an optional's empty case
	// spelled by name -- `.none`, `Optional<T>.none` -- each mapped to
	// the optional it makes.
	OptionalSomes map[*ast.CallExpr]types.Type
	OptionalNones map[ast.Expr]types.Type

	// RawValues is the literal each enum case declares as its raw value,
	// where it declares one: `case spades = "S"`, `case high = 2.25`.
	RawValues map[*types.EnumCase]ast.Expr

	// KeyPathClosures is the closure a key path used as a function is:
	// `\.name` where a (T) -> U is wanted is `{ $0.name }`.
	KeyPathClosures map[*ast.KeyPathExpr]*ast.ClosureExpr

	// KeyPathValues is each key path used as a value: its KeyPath type,
	// and the closures that read and -- where the path can be written --
	// write through it. KeyPathReads is each `x[keyPath: k]`, with k's
	// type.
	KeyPathValues map[*ast.KeyPathExpr]*KeyPathValue
	KeyPathReads  map[*ast.SubscriptExpr]types.Type

	// SelfVars is each `self` written in a closure that captured
	// `[weak self]` or `[unowned self]`: the closure's own self it names.
	SelfVars map[*ast.SelfExpr]*VarSymbol

	// CoreAlgorithms is the core's source with bodies, checked in this
	// Info: what the program uses of it is lowered from here.
	CoreAlgorithms *ast.File

	// Iterations are the for-in loops over sequences of the program's own.
	Iterations map[*ast.ForInStmt]*Iteration

	// Subscripts are the uses of subscripts types declare: `grid[1, 2]`.
	// SubscriptDecls is the subscript each declaration declares.
	Subscripts     map[*ast.SubscriptExpr]*SubscriptRef
	SubscriptDecls map[*ast.SubscriptDecl]*types.Subscript

	// OptionalCompares are the `==` and `!=` whose operands are optionals,
	// or an optional and a value, mapped to the type the payloads are
	// compared as; the operator the payloads use is recorded for the
	// expression as any operator is.
	OptionalCompares map[*ast.BinaryExpr]types.Type

	// CastTargets is the type each `x is T` names.
	CastTargets map[*ast.CastExpr]types.Type

	// Layouts are the reads of MemoryLayout<T>.size, .stride and
	// .alignment. T may be a generic parameter here, so the number is
	// the lowering's to work out, once T is known.
	Layouts map[*ast.MemberExpr]Layout

	// TypeOfs are the `type(of: x)` calls, mapped to x's static type: the
	// type's metatype, or for a class the object's dynamic one.
	TypeOfs map[*ast.CallExpr]types.Type

	// ChainRoots are the outermost steps of optional chains, whose Types
	// are the optionals they answer; ChainInner is what each gives when the
	// chain has a value all the way along.
	ChainRoots map[ast.Expr]bool
	ChainInner map[ast.Expr]types.Type

	// CStrings are the arguments that are a String passed where a C string
	// is wanted -- an UnsafePointer<CChar>, <UInt8> or UnsafeRawPointer,
	// optional or not -- mapped to the pointer type they become. Swift makes
	// that conversion at a call and nowhere else: the pointer is to a
	// NUL-terminated copy that lives until the call returns.
	CStrings map[ast.Expr]types.Type

	// Unwrapped are the expressions whose value is an implicitly unwrapped
	// optional used as the type it wraps -- a `T!` result passed where a T
	// is wanted -- mapped to the optional they are unwrapped from. Types
	// holds the wrapped type for them; the unwrap traps on nil, as `!`.
	Unwrapped map[ast.Expr]types.Type

	// DefaultFiles is the file each default expression was written in,
	// which for an imported function is a file of another module.
	DefaultFiles map[ast.Expr]*token.File

	// Imported maps imported symbols to declaring module names.
	Imported map[Symbol]string

	// ImportedUnits maps imported symbols to interface AST files.
	ImportedUnits map[Symbol]*token.File

	// ImportedFiles are the source files of the imported modules. Their
	// generic declarations are checked in this module, because this module
	// is where they are specialized (see checkImportedGenerics), and the
	// generator reads the bodies from here.
	ImportedFiles []*ast.File

	// ImportedTypes maps imported types to declaring module names.
	ImportedTypes map[types.Type]string

	// SwiftModules tracks imported swiftc interface module names.
	SwiftModules map[string]bool

	// MainActor is the nominal types declared @MainActor, by their
	// underlying type: every member of one is isolated to the main
	// thread, in the declaration and in its extensions.
	MainActor map[types.Type]bool
	// Wrappers is the types declared @propertyWrapper; WrapperInits the
	// call of a wrapper's initializer a wrapped property's storage starts
	// as, by the property's binding: `Clamped(wrappedValue: 5, 0...10)`.
	Wrappers     map[types.Type]bool
	// DynamicMembers is the types declared @dynamicMemberLookup, whose
	// members nothing declares are subscripts with dynamicMember:.
	DynamicMembers map[types.Type]bool
	WrapperInits map[*ast.PatternBinding]*ast.CallExpr

	// Diagnostics holds all warnings and errors produced during analysis.
	Diagnostics []token.Diagnostic
}

// MethodRef represents a resolved method and its declaring type.
type MethodRef struct {
	Recv   types.Type
	Method *types.Method
}

// An Iteration is how a for-in goes through a value of a type that is a
// Sequence of the program's own: through the iterator makeIterator()
// makes, or the value itself where it is its own iterator, by next().
type Iteration struct {
	// MakeIterator is nil where the sequence is its own iterator.
	MakeIterator *MethodRef
	Iterator     types.Type
	Next         *MethodRef
	Element      types.Type
}

// A SubscriptRef is a subscript a type declares, used on a value of it
// -- or on the type itself, for a static one.
// A CallSite is where a call is, for the #line and #function a parameter
// it leaves to its default takes: the function the call is made in, as
// #function spells it, and its position.
type CallSite struct {
	Func string
	Pos  token.Pos
}

type SubscriptRef struct {
	Recv      types.Type
	Subscript *types.Subscript
	// Subst is what the subscript's own generic parameters are at this
	// use, where it has any.
	Subst map[*types.TypeParam]types.Type
}

// NewInfo allocates an empty Info container.
func NewInfo() *Info {
	return &Info{
		Types:            make(map[ast.Expr]types.Type),
		Defs:             make(map[*ast.Ident]Symbol),
		Uses:             make(map[*ast.Ident]Symbol),
		Scopes:           make(map[ast.Node]*Scope),
		Folded:           make(map[*ast.SequenceExpr]ast.Expr),
		Operators:        make(map[ast.Expr]Symbol),
		PatternTypes:     make(map[ast.Pattern]types.Type),
		CoreTypes:        make(map[types.Type]bool),
		LiteralInits:     make(map[ast.Expr]*LiteralInit),
		Captures:         make(map[*ast.CaptureItem]*Capture),
		Autoclosures:     make(map[ast.Expr]*types.Signature),
		ArraySequences:   make(map[*ast.CallExpr]*Iteration),
		OperatorCalls:    make(map[ast.Expr]*ast.CallExpr),
		OperatorSpecs:    make(map[ast.Expr]Specialization),
		ArrayRepeats:     make(map[*ast.CallExpr]bool),
		CoreCalls:        make(map[*ast.CallExpr]*ast.CallExpr),
		MainActor:        make(map[types.Type]bool),
		Wrappers:         make(map[types.Type]bool),
		DynamicMembers:   make(map[types.Type]bool),
		CallSites:        make(map[*ast.CallExpr]CallSite),
		PatternMatches:   make(map[*ast.ExprPattern]*FuncSymbol),
		WrapperInits:     make(map[*ast.PatternBinding]*ast.CallExpr),
		Values:           make(map[ast.Node]Value),
		Methods:          make(map[*ast.MemberExpr]*MethodRef),
		Extensions:       make(map[*ast.ExtensionDecl]types.Type),
		Builtins:         make(map[string]*BuiltinMembers),
		ImplicitSelf:     make(map[ast.Expr]ast.Expr),
		Receivers:        make(map[*ast.FuncDecl]types.Type),
		FieldDefaults:    make(map[*types.Field]ast.Expr),
		Specializations:  make(map[*ast.CallExpr]Specialization),
		Defaults:         make(map[*types.Param]ast.Expr),
		CStrings:         make(map[ast.Expr]types.Type),
		Layouts:          make(map[*ast.MemberExpr]Layout),
		TypeOfs:          make(map[*ast.CallExpr]types.Type),
		Unwrapped:        make(map[ast.Expr]types.Type),
		RawInits:         make(map[*ast.CallExpr]*types.Enum),
		Inits:            make(map[*ast.CallExpr]*types.Signature),
		OperatorMethods:  make(map[ast.Expr]*MethodRef),
		ImplicitMethods:  make(map[*ast.ImplicitMemberExpr]*MethodRef),
		DerivedOperators: make(map[ast.Expr]*DerivedOperator),
		EmptyCollections: make(map[*ast.CallExpr]types.Type),
		ArrayCopies:      make(map[*ast.CallExpr]types.Type),
		OptionalSomes:    make(map[*ast.CallExpr]types.Type),
		OptionalNones:    make(map[ast.Expr]types.Type),
		RawValues:        make(map[*types.EnumCase]ast.Expr),
		KeyPathClosures:  make(map[*ast.KeyPathExpr]*ast.ClosureExpr),
		SelfVars:         make(map[*ast.SelfExpr]*VarSymbol),
		KeyPathValues:    make(map[*ast.KeyPathExpr]*KeyPathValue),
		KeyPathReads:     make(map[*ast.SubscriptExpr]types.Type),
		Iterations:       make(map[*ast.ForInStmt]*Iteration),
		Subscripts:       make(map[*ast.SubscriptExpr]*SubscriptRef),
		SubscriptDecls:   make(map[*ast.SubscriptDecl]*types.Subscript),
		OptionalCompares: make(map[*ast.BinaryExpr]types.Type),
		CastTargets:      make(map[*ast.CastExpr]types.Type),
		ChainRoots:       make(map[ast.Expr]bool),
		ChainInner:       make(map[ast.Expr]types.Type),
		DefaultFiles:     make(map[ast.Expr]*token.File),
		Imported:         make(map[Symbol]string),
		ImportedUnits:    make(map[Symbol]*token.File),
		ImportedTypes:    make(map[types.Type]string),
		SwiftModules:     make(map[string]bool),
		Diagnostics:      nil,
	}
}

// TypeOf returns the type of expression e, or nil if unrecorded.
func (info *Info) TypeOf(e ast.Expr) types.Type {
	if info.Types == nil {
		return nil
	}
	return info.Types[e]
}

// SymbolOf returns the symbol defined or used by ident, or nil.
func (info *Info) SymbolOf(id *ast.Ident) Symbol {
	if s, ok := info.Defs[id]; ok {
		return s
	}
	if s, ok := info.Uses[id]; ok {
		return s
	}
	return nil
}

// ScopeOf returns the Scope associated with node n, or nil.
func (info *Info) ScopeOf(n ast.Node) *Scope {
	if info.Scopes == nil {
		return nil
	}
	return info.Scopes[n]
}

// RangeOf is the bound type of t where t is one of the core's ranges,
// Range<Bound> or ClosedRange<Bound>, and which of the two it is.
func (info *Info) RangeOf(t types.Type) (bound types.Type, closed, ok bool) {
	bound, name, ok := info.AnyRangeOf(t)
	switch name {
	case "Range":
		return bound, false, ok
	case "ClosedRange":
		return bound, true, ok
	}
	return nil, false, false
}

// AnyRangeOf is the bound type of t where t is any of the core's ranges,
// one-sided ones too, and the range's name: "Range", "ClosedRange",
// "PartialRangeUpTo", "PartialRangeThrough" or "PartialRangeFrom".
// Inside the core's own extensions of them, self is the range of Bound.
func (info *Info) AnyRangeOf(t types.Type) (bound types.Type, name string, ok bool) {
	base := t
	if inst, isInst := t.(*types.GenericInstance); isInst {
		if len(inst.Args) != 1 {
			return nil, "", false
		}
		base, bound = inst.Base, inst.Args[0]
	}
	if base == nil || !info.CoreTypes[base] {
		return nil, "", false
	}
	st, isStruct := base.Underlying().(*types.Struct)
	if !isStruct || len(st.TypeParams) != 1 {
		return nil, "", false
	}
	if bound == nil {
		bound = st.TypeParams[0]
	}
	switch st.Name {
	case "Range", "ClosedRange", "PartialRangeUpTo", "PartialRangeThrough", "PartialRangeFrom":
		return bound, st.Name, true
	}
	return nil, "", false
}

// A Capture is a name a capture list binds and the value it is bound to,
// evaluated where the closure is made.
// A KeyPathValue is a key path used as a value.
type KeyPathValue struct {
	Type     types.Type
	Get, Set *ast.ClosureExpr
}

type Capture struct {
	Sym   *VarSymbol
	Value ast.Expr
}

// A LiteralInit is how a literal becomes a value of a type expressible by
// it: the literal, as the type the initializer takes, is passed to Init.
type LiteralInit struct {
	// Type is the type made; the literal's own type may be an optional
	// of it, which the value made is then wrapped in.
	Type types.Type
	// Init is the initializer: init(integerLiteral:), init(stringLiteral:)
	// and the rest. Its one parameter's type is what the literal is.
	Init *types.Signature
}

// FoldedOf returns the folded tree for a SequenceExpr, or the sequence itself if not folded.
func (info *Info) FoldedOf(seq *ast.SequenceExpr) ast.Expr {
	if f, ok := info.Folded[seq]; ok {
		return f
	}
	return seq
}

// Specialization records generic type parameters and concrete argument types.
type Specialization struct {
	Params []*types.TypeParam
	Args   []types.Type
}

// Subst is the substitution this specialization applies.
func (s Specialization) Subst() map[*types.TypeParam]types.Type {
	out := make(map[*types.TypeParam]types.Type, len(s.Params))
	for i, p := range s.Params {
		if i < len(s.Args) {
			out[p] = s.Args[i]
		}
	}
	return out
}

// Empty reports whether nothing was substituted.
func (s Specialization) Empty() bool { return len(s.Params) == 0 }

// setConditions records what a member's extension asks of its type's
// parameters.
func (info *Info) setConditions(member any, conds []memberCondition) {
	if info.conditions == nil {
		info.conditions = map[any][]memberCondition{}
	}
	info.conditions[member] = conds
}

// A Layout is one read of MemoryLayout<Of>: its size, stride or
// alignment, named by Kind.
type Layout struct {
	Of   types.Type
	Kind string
}
