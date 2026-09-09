package analyzer

import (
	"fmt"

	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/token"
	"github.com/vertex-language/vsc/types"
)

// Symbol is something a name denotes. Every name a program resolves
// ends at one of these, and Info.Uses is the map from the one to the
// other.
type Symbol interface {
	Name() string
	Type() types.Type
	Pos() token.Pos
	Decl() ast.Decl
	String() string
}

// VarSymbol is a variable, a constant, or a parameter. It carries
// what the checker learns about it as the body is read: whether it
// has been initialized, and whether it has been consumed.
type VarSymbol struct {
	name          string
	typ           types.Type
	pos           token.Pos
	decl          ast.Decl
	isConst       bool
	ownership     types.OwnershipKind
	isInitialized bool
	isConsumed    bool
}

func NewVar(name string, typ types.Type, pos token.Pos, isConst bool, ownership types.OwnershipKind) *VarSymbol {
	return &VarSymbol{
		name:          name,
		typ:           typ,
		pos:           pos,
		isConst:       isConst,
		ownership:     ownership,
		isInitialized: true,
		isConsumed:    false,
	}
}

func (v *VarSymbol) Name() string                   { return v.name }
func (v *VarSymbol) Type() types.Type               { return v.typ }
func (v *VarSymbol) Pos() token.Pos                 { return v.pos }
func (v *VarSymbol) Decl() ast.Decl                 { return v.decl }
func (v *VarSymbol) SetDecl(d ast.Decl)             { v.decl = d }
func (v *VarSymbol) IsConst() bool                  { return v.isConst }
func (v *VarSymbol) Ownership() types.OwnershipKind { return v.ownership }
func (v *VarSymbol) IsInitialized() bool            { return v.isInitialized }
func (v *VarSymbol) SetInitialized(init bool)       { v.isInitialized = init }
func (v *VarSymbol) IsConsumed() bool               { return v.isConsumed }
func (v *VarSymbol) SetConsumed(consumed bool)      { v.isConsumed = consumed }
func (v *VarSymbol) String() string {
	kind := "var"
	if v.isConst {
		kind = "let"
	}
	return fmt.Sprintf("%s %s: %s", kind, v.name, v.typ)
}

// FuncSymbol is a function or a method.
type FuncSymbol struct {
	name   string
	sig    *types.Signature
	pos    token.Pos
	decl   ast.Decl
	access Access

	// others are the declarations that share this name. Swift lets
	// them, as long as they do not share a signature, so a name in
	// scope is not one function but a set of them and a call picks
	// from it.
	others []*FuncSymbol
}

func NewFunc(name string, sig *types.Signature, pos token.Pos) *FuncSymbol {
	return &FuncSymbol{name: name, sig: sig, pos: pos}
}

func (f *FuncSymbol) Name() string                { return f.name }
func (f *FuncSymbol) Type() types.Type            { return f.sig }
func (f *FuncSymbol) Signature() *types.Signature { return f.sig }
func (f *FuncSymbol) Pos() token.Pos              { return f.pos }
func (f *FuncSymbol) Decl() ast.Decl              { return f.decl }
func (f *FuncSymbol) SetDecl(d ast.Decl)          { f.decl = d }

// Access is how far the declaration can be seen. It is what decides
// the linkage of the symbol the function is given, and so whether
// anything outside this module can call it.
func (f *FuncSymbol) Access() Access     { return f.access }
func (f *FuncSymbol) SetAccess(a Access) { f.access = a }
func (f *FuncSymbol) String() string {
	return fmt.Sprintf("func %s%s", f.name, f.sig)
}

// Overloads returns every declaration of this name, this one first.
func (f *FuncSymbol) Overloads() []*FuncSymbol {
	return append([]*FuncSymbol{f}, f.others...)
}

// AddOverload records another declaration of the same name.
func (f *FuncSymbol) AddOverload(g *FuncSymbol) { f.others = append(f.others, g) }

// TypeNameSymbol is a name that denotes a type. In expression
// position it is a metatype, which is what makes `S(…)` a call.
type TypeNameSymbol struct {
	name string
	typ  types.Type
	pos  token.Pos
	decl ast.Decl
}

func NewTypeName(name string, typ types.Type, pos token.Pos) *TypeNameSymbol {
	return &TypeNameSymbol{name: name, typ: typ, pos: pos}
}

func (t *TypeNameSymbol) Name() string       { return t.name }
func (t *TypeNameSymbol) Type() types.Type   { return t.typ }
func (t *TypeNameSymbol) Pos() token.Pos     { return t.pos }
func (t *TypeNameSymbol) Decl() ast.Decl     { return t.decl }
func (t *TypeNameSymbol) SetDecl(d ast.Decl) { t.decl = d }
func (t *TypeNameSymbol) String() string {
	return fmt.Sprintf("type %s = %s", t.name, t.typ)
}

// EnumCaseSymbol is one case of an enum, declared in the enum's own
// scope so that a member of the type finds it unqualified.
type EnumCaseSymbol struct {
	name           string
	enumType       types.Type
	associatedType types.Type
	pos            token.Pos
	decl           ast.Decl
}

func NewEnumCase(name string, enumType, assocType types.Type, pos token.Pos) *EnumCaseSymbol {
	return &EnumCaseSymbol{
		name:           name,
		enumType:       enumType,
		associatedType: assocType,
		pos:            pos,
	}
}

func (ec *EnumCaseSymbol) Name() string               { return ec.name }
func (ec *EnumCaseSymbol) Type() types.Type           { return ec.enumType }
func (ec *EnumCaseSymbol) AssociatedType() types.Type { return ec.associatedType }
func (ec *EnumCaseSymbol) Pos() token.Pos             { return ec.pos }
func (ec *EnumCaseSymbol) Decl() ast.Decl             { return ec.decl }
func (ec *EnumCaseSymbol) String() string {
	if ec.associatedType != nil {
		return fmt.Sprintf("case %s(%s)", ec.name, ec.associatedType)
	}
	return fmt.Sprintf("case %s", ec.name)
}
