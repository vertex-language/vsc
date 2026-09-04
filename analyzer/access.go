package analyzer

import "github.com/vertex-language/vsc/ast"

// An Access is how far a declaration can be seen.
//
// Swift has six, and they are ordered: everything a `private`
// declaration is visible to, a `public` one is visible to as well. The
// order is what makes them comparable, which is how the rule that a
// declaration may not be more visible than its own type is checked.
type Access int

const (
	// Private is visible inside the enclosing declaration.
	Private Access = iota
	// FilePrivate is visible inside the file it is written in.
	FilePrivate
	// Internal is visible inside the module, and is what a
	// declaration that says nothing gets.
	Internal
	// Package is visible inside the package the module belongs to.
	Package
	// Public is visible outside the module.
	Public
	// Open is Public, and may also be subclassed or overridden
	// outside the module. It is the only level that says something
	// about inheritance rather than only about visibility.
	Open
)

func (a Access) String() string {
	switch a {
	case Private:
		return "private"
	case FilePrivate:
		return "fileprivate"
	case Package:
		return "package"
	case Public:
		return "public"
	case Open:
		return "open"
	}
	return "internal"
}

// accessOf reads a declaration's modifiers. A declaration that says
// nothing is internal: visible across the module it is written in and
// nowhere else, which is the default Swift chose so that making
// something part of an interface is a decision someone had to write
// down.
func (c *checker) accessOf(mods []*ast.Modifier) Access {
	for _, m := range mods {
		if m.Name == nil {
			continue
		}
		switch m.Name.Text(c.file) {
		case "private":
			return Private
		case "fileprivate":
			return FilePrivate
		case "internal":
			return Internal
		case "package":
			return Package
		case "public":
			return Public
		case "open":
			return Open
		}
	}
	return Internal
}
