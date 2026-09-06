package analyzer

import (
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/token"
)

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

// Where a declaration was written, and how far it can be seen from.
//
// Kept beside the symbols rather than on them because only a function
// carries an access level today, and because Access's zero value is
// Private -- a field added to a symbol type would make every symbol
// nobody remembered to stamp invisible outside its own file. A map
// says nothing about the symbols that are not in it, which is the
// right default for a partial answer.
type declSite struct {
	file   *token.File
	access Access
}

// declaredHere records a top-level declaration's file and access.
func (c *checker) declaredHere(sym Symbol, access Access) {
	if sym == nil || c.file == nil {
		return
	}
	if c.declSites == nil {
		c.declSites = make(map[Symbol]declSite)
	}
	c.declSites[sym] = declSite{file: c.file, access: access}
}

// checkAccess reports a use of a name the file it is written in
// cannot see.
//
// Only the file-scoped levels are enforceable inside one module:
// internal is the module and every file here is in it, and public is
// wider still. `private` at file scope means the same as
// `fileprivate` -- Swift scopes it to the enclosing declaration, and
// for a top-level declaration that is the file.
//
// A symbol with no recorded site is not restricted. That is what
// makes this safe to add a declaration at a time rather than all at
// once: what is not stamped is not checked, instead of being
// invisible.
func (c *checker) checkAccess(pos token.Pos, name string, sym Symbol) {
	site, ok := c.declSites[sym]
	if !ok || c.file == nil || site.file == nil {
		return
	}
	if site.access != Private && site.access != FilePrivate {
		return
	}
	if site.file == c.file {
		return
	}
	c.errorf(pos, "'%s' is inaccessible due to '%s' protection level", name, site.access)
}
