package analyzer

import (
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/token"
)

// Access represents declaration visibility and access control levels.
type Access int

const (
	Private Access = iota
	FilePrivate
	Internal
	Package
	Public
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

// accessOf parses declaration access modifiers, defaulting to Internal.
// hasModifier reports whether mods include the modifier named name.
func (c *checker) hasModifier(mods []*ast.Modifier, name string) bool {
	for _, m := range mods {
		if m.Name != nil && m.Name.Text(c.file) == name {
			return true
		}
	}
	return false
}

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

// declSite records declaration source file and access control level.
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

// checkAccess validates that referenced symbol is accessible from the current file.
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

// exported reports whether an access level puts a declaration in its
// module's interface.
func exported(a Access) bool { return a == Public || a == Open }
