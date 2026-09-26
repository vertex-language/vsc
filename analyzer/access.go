package analyzer

import (
	"strings"

	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/token"
	"github.com/vertex-language/vsc/types"
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

// hasAccessModifier reports whether mods state an access level.
func (c *checker) hasAccessModifier(mods []*ast.Modifier) bool {
	for _, m := range mods {
		if m.Name == nil {
			continue
		}
		switch m.Name.Text(c.file) {
		case "private", "fileprivate", "internal", "package", "public", "open":
			return true
		}
	}
	return false
}

// memberAccessOf is a member's access: its own, or else the one its
// extension states for every member (`public extension`).
func (c *checker) memberAccessOf(mods []*ast.Modifier) Access {
	if !c.hasAccessModifier(mods) && c.extAccess != nil {
		return *c.extAccess
	}
	return c.accessOf(mods)
}

// visibleOutside reports whether another package may name a declaration
// of this access. `package` is the repository's, which a Vertex package
// shares with its siblings.
func visibleOutside(a Access) bool { return a >= Package }

// readingSourcePackage reports whether the declarations being read are an
// imported Vertex package's source -- which declares everything, internal
// helpers too, because its generic and @inlinable bodies are compiled
// here -- rather than an interface, which holds only what is public. The
// built-in gpu module is the compiler's own, and the calls it writes for a
// kernel's Launch reach its internals.
func (c *checker) readingSourcePackage() bool {
	if c.importing == "" || c.importing == "Swift" || c.importing == "gpu" || c.file == nil {
		return false
	}
	return strings.HasSuffix(c.file.Name(), ".vs")
}

// hideImportedMethod records a method of an imported source package that
// the program may not call: one that is not public.
func (c *checker) hideImportedMethod(m *types.Method, sym *FuncSymbol, access Access) {
	if visibleOutside(access) || !c.readingSourcePackage() {
		return
	}
	if c.hidden == nil {
		c.hidden = map[Symbol]Access{}
		c.hiddenMethods = map[*types.Method]Access{}
	}
	c.hiddenMethods[m] = access
	c.hidden[sym] = access
}

// hideImported records a module-scope declaration of an imported source
// package that the program may not name: every overload of it that is
// not public. A C++ export of a folder that has Vertex of its own is one:
// its bindings are written without `public`, for the package's Vertex.
func (c *checker) hideImported(imp Import, sym Symbol, unitOf map[ast.Decl]*token.File) {
	if imp.Name == "gpu" {
		return
	}
	var syms []Symbol
	if fn, ok := sym.(*FuncSymbol); ok {
		for _, o := range fn.Overloads() {
			syms = append(syms, o)
		}
	} else {
		syms = []Symbol{sym}
	}
	prev := c.file
	defer func() { c.file = prev }()
	for _, s := range syms {
		d := s.Decl()
		unit := unitOf[d]
		if d == nil || unit == nil || !strings.HasSuffix(unit.Name(), ".vs") {
			continue
		}
		c.file = unit
		if a := c.accessOf(declMods(d)); !visibleOutside(a) {
			if c.hidden == nil {
				c.hidden = map[Symbol]Access{}
				c.hiddenMethods = map[*types.Method]Access{}
			}
			c.hidden[s] = a
		}
	}
}

// declMods is a declaration's modifiers.
func declMods(d ast.Decl) []*ast.Modifier {
	switch d := d.(type) {
	case *ast.FuncDecl:
		return d.Mods
	case *ast.StructDecl:
		return d.Mods
	case *ast.ClassDecl:
		return d.Mods
	case *ast.ActorDecl:
		return d.Mods
	case *ast.EnumDecl:
		return d.Mods
	case *ast.ProtocolDecl:
		return d.Mods
	case *ast.TypealiasDecl:
		return d.Mods
	case *ast.VarDecl:
		return d.Mods
	}
	return nil
}

// checkImportedAccess holds the program to its imports' access control,
// once every name is resolved: a name, a module member or a method that
// resolved to an imported package's non-public declaration is refused, as
// if it had not been found -- which is what an interface, holding only the
// public declarations, would have said.
func (c *checker) checkImportedAccess() {
	if len(c.hidden) == 0 {
		return
	}
	prev := c.file
	defer func() { c.file = prev }()
	for _, f := range c.files {
		if f.Unit == nil {
			continue
		}
		c.file = f.Unit
		reported := map[token.Pos]bool{}
		report := func(at *ast.Ident, a Access) {
			if at == nil || reported[at.Pos()] {
				return
			}
			reported[at.Pos()] = true
			c.errorf(at.Pos(), "'%s' is inaccessible due to '%s' protection level", at.Text(c.file), a)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.Ident:
				if a, ok := c.hidden[c.info.Uses[n]]; ok {
					report(n, a)
				}
			case *ast.MemberExpr:
				if ref := c.info.Methods[n]; ref != nil {
					for m := ref.Method; m != nil; m = m.Origin {
						if a, ok := c.hiddenMethods[m]; ok {
							report(n.Name, a)
							break
						}
					}
				}
			}
			return true
		})
	}
}

// checkHiddenType refuses a type name, written in the program, that
// resolved to an imported package's non-public type. A type is not a
// use the final pass sees, so it is checked where it is resolved.
func (c *checker) checkHiddenType(pos token.Pos, name string, sym Symbol) {
	if c.importing != "" || sym == nil {
		return
	}
	if a, ok := c.hidden[sym]; ok {
		c.errorf(pos, "'%s' is inaccessible due to '%s' protection level", name, a)
	}
}
