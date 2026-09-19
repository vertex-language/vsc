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

	// CastTargets is the type each `x is T` names.
	CastTargets map[*ast.CastExpr]types.Type

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

	// ImportedTypes maps imported types to declaring module names.
	ImportedTypes map[types.Type]string

	// SwiftModules tracks imported swiftc interface module names.
	SwiftModules map[string]bool

	// Diagnostics holds all warnings and errors produced during analysis.
	Diagnostics []token.Diagnostic
}

// MethodRef represents a resolved method and its declaring type.
type MethodRef struct {
	Recv   types.Type
	Method *types.Method
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
