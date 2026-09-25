package ast

import "github.com/vertex-language/vsc/token"

// HasAttr reports whether attrs holds @name, reading names from file.
func HasAttr(attrs []*Attr, file *token.File, name string) bool {
	for _, a := range attrs {
		if a == nil || file == nil {
			continue
		}
		if id, ok := a.Name.(*IdentType); ok && id.Name != nil && id.Name.Text(file) == name {
			return true
		}
	}
	return false
}

// Inlinable is Swift's @inlinable: a public function whose body is part
// of its module's interface, so that a client can compile it itself --
// to inline or specialize it, and to run it on a device, where a call
// into another module's object code is impossible.
const Inlinable = "inlinable"

// IsInlinable reports whether d is a module-scope function marked
// @inlinable, with a body, and not generic: the kind an interface keeps
// the body of and a client compiles.
func IsInlinable(d *FuncDecl, file *token.File) bool {
	return d != nil && d.Recv == nil && d.Body != nil && d.Generics == nil &&
		HasAttr(d.Attrs, file, Inlinable)
}

// InlinableMember reports whether n is a method or initializer of a type
// marked @inlinable, with a body and no type parameters of its own.
func InlinableMember(n Node, file *token.File) bool {
	switch m := n.(type) {
	case *FuncDecl:
		return m.Body != nil && m.Generics == nil && HasAttr(m.Attrs, file, Inlinable)
	case *InitDecl:
		return m.Body != nil && m.Generics == nil && HasAttr(m.Attrs, file, Inlinable)
	}
	return false
}
