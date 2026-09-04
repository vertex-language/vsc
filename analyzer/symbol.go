package analyzer

import (
	"fmt"

	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/token"
	"github.com/vertex-language/vsc/types"
)

// Symbol represents a declared language entity (variable, function, type, etc.).
type Symbol interface {
	Name() string
	Type() types.Type
	Pos() token.Pos
	Decl() ast.Decl
	String() string
}

// VarSymbol represents a variable, constant, or parameter.
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
func (v *VarSymbol) SetType(t types.Type)           { v.typ = t }
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

// FuncSymbol represents a function, method, or initializer.
type FuncSymbol struct {
	name string
	sig  *types.Signature
	pos  token.Pos
	decl ast.Decl
}

func NewFunc(name string, sig *types.Signature, pos token.Pos) *FuncSymbol {
	return &FuncSymbol{name: name, sig: sig, pos: pos}
}

func (f *FuncSymbol) Name() string                      { return f.name }
func (f *FuncSymbol) Type() types.Type                  { return f.sig }
func (f *FuncSymbol) Signature() *types.Signature       { return f.sig }
func (f *FuncSymbol) SetSignature(sig *types.Signature) { f.sig = sig }
func (f *FuncSymbol) Pos() token.Pos                    { return f.pos }
func (f *FuncSymbol) Decl() ast.Decl                    { return f.decl }
func (f *FuncSymbol) SetDecl(d ast.Decl)                { f.decl = d }
func (f *FuncSymbol) String() string {
	return fmt.Sprintf("func %s%s", f.name, f.sig)
}

// TypeNameSymbol represents a named type (struct, class, enum, protocol, typealias).
type TypeNameSymbol struct {
	name string
	typ  types.Type
	pos  token.Pos
	decl ast.Decl
}

func NewTypeName(name string, typ types.Type, pos token.Pos) *TypeNameSymbol {
	return &TypeNameSymbol{name: name, typ: typ, pos: pos}
}

func (t *TypeNameSymbol) Name() string          { return t.name }
func (t *TypeNameSymbol) Type() types.Type      { return t.typ }
func (t *TypeNameSymbol) SetType(tp types.Type) { t.typ = tp }
func (t *TypeNameSymbol) Pos() token.Pos        { return t.pos }
func (t *TypeNameSymbol) Decl() ast.Decl        { return t.decl }
func (t *TypeNameSymbol) SetDecl(d ast.Decl)    { t.decl = d }
func (t *TypeNameSymbol) String() string {
	return fmt.Sprintf("type %s = %s", t.name, t.typ)
}

// EnumCaseSymbol represents a single case of an enumeration.
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

// PackageSymbol represents an imported module or package.
type PackageSymbol struct {
	name  string
	scope *Scope
}

func NewPackage(name string, scope *Scope) *PackageSymbol {
	return &PackageSymbol{name: name, scope: scope}
}

func (p *PackageSymbol) Name() string     { return p.name }
func (p *PackageSymbol) Type() types.Type { return nil }
func (p *PackageSymbol) Pos() token.Pos   { return token.NoPos }
func (p *PackageSymbol) Decl() ast.Decl   { return nil }
func (p *PackageSymbol) Scope() *Scope    { return p.scope }
func (p *PackageSymbol) String() string   { return fmt.Sprintf("package %s", p.name) }
