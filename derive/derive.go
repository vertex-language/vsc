// Package derive writes the declarations Swift makes for a type that asks
// for a conformance without implementing it.
//
// A struct that says it is Equatable -- or Comparable, or Hashable, which
// are Equatable too -- and writes no `==` of its own gets one from swiftc:
// `static func __derived_struct_equals(_: Self, _: Self) -> Bool`,
// comparing every stored property in the order they are declared. One that
// says it is Hashable and writes no `hash(into:)` gets that too, combining
// the same properties; so does an enum of cases alone, from its case. This
// package writes those functions as source, in an extension of the type,
// and the extension is compiled with the module like any other file.
// Checking, operator lookup and lowering then treat them as the ordinary
// functions they are, and `==`'s symbol is the one swiftc gives the function
// it makes.
//
// It reads syntax only. A type whose stored properties are not all
// Equatable gets a function that does not check, and the diagnostic names
// the property, as swiftc's does.
package derive

import (
	"fmt"
	"strings"

	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/token"
)

// EqualsName is the name of the `==` swiftc derives for a struct.
const EqualsName = "__derived_struct_equals"

// EnumEqualsName is the name of the `==` swiftc derives for an enum whose
// cases carry values. One of cases alone compares its tags, and needs none.
const EnumEqualsName = "__derived_enum_equals"

// equatables are the protocols that make a type Equatable.
var equatables = map[string]bool{"Equatable": true, "Comparable": true, "Hashable": true}

// nominal is a struct or enum found in the files, with what deriving needs
// of it.
type nominal struct {
	path      string // Outer.Inner
	isEnum    bool
	public    bool
	generic   bool
	fields    []string   // a struct's stored properties
	cases     []enumCase // an enum's cases
	payload   bool       // an enum case carries a value
	equatable bool       // the declaration or an extension says so
	hashable  bool
	hasEquals bool // a `==` taking it, or the derived function, is declared
	hasHash   bool // a hash(into:) is declared
}

// enumCase is one case of an enum: its name, and how many values it
// carries.
type enumCase struct {
	name  string
	arity int
}

// Source is the file of derived declarations for a module's files, or nil
// when there is nothing to derive. The package clause of the files, if
// there is one, is repeated so the file belongs to the same package.
func Source(files []*ast.File) []byte {
	found := map[string]*nominal{}
	var order []string
	pkg := ""

	var visitType func(unit *token.File, n ast.Node, outer string)
	var visitMembers func(unit *token.File, body *ast.MemberBlock, owner *nominal, path string)

	visitType = func(unit *token.File, n ast.Node, outer string) {
		var name *ast.Ident
		var body *ast.MemberBlock
		var mods []*ast.Modifier
		var inherit *ast.InheritanceClause
		generic, isEnum := false, false
		switch x := n.(type) {
		case *ast.StructDecl:
			name, body, mods, inherit, generic = x.Name, x.Body, x.Mods, x.Inherit, x.Generics != nil
		case *ast.EnumDecl:
			name, body, mods, inherit, generic, isEnum = x.Name, x.Body, x.Mods, x.Inherit, x.Generics != nil, true
		case *ast.ClassDecl:
			// A class derives nothing, but types nested in one may.
			if x.Name != nil {
				visitMembers(unit, x.Body, nil, join(outer, x.Name.Name(unit)))
			}
			return
		}
		if name == nil {
			return
		}
		path := join(outer, name.Name(unit))
		t := found[path]
		if t == nil {
			t = &nominal{path: path, isEnum: isEnum}
			found[path] = t
			order = append(order, path)
		}
		t.public = hasModifier(unit, mods, "public") || hasModifier(unit, mods, "open")
		t.generic = generic
		eq, hash := says(unit, inherit)
		t.equatable, t.hashable = t.equatable || eq, t.hashable || hash
		visitMembers(unit, body, t, path)
	}

	visitMembers = func(unit *token.File, body *ast.MemberBlock, owner *nominal, path string) {
		if body == nil {
			return
		}
		for _, m := range body.Members {
			switch x := m.(type) {
			case *ast.EnumCaseDecl:
				if owner == nil {
					continue
				}
				for _, el := range x.Elements {
					if el.Name != nil {
						owner.cases = append(owner.cases, enumCase{name: el.Name.Name(unit), arity: len(el.Params)})
					}
					if el.Lparen.IsValid() {
						owner.payload = true
					}
				}
			case *ast.VarDecl:
				if owner == nil || owner.isEnum || hasModifier(unit, x.Mods, "static") || hasModifier(unit, x.Mods, "class") {
					continue
				}
				for _, b := range x.Bindings {
					if name, ok := stored(unit, b); ok {
						owner.fields = append(owner.fields, name)
					}
				}
			case *ast.FuncDecl:
				if owner != nil && declaresEquals(unit, x, owner.path) {
					owner.hasEquals = true
				}
				if owner != nil && declaresHash(unit, x) {
					owner.hasHash = true
				}
			case *ast.StructDecl, *ast.EnumDecl, *ast.ClassDecl:
				visitType(unit, x, path)
			}
		}
	}

	type extension struct {
		unit *token.File
		d    *ast.ExtensionDecl
	}
	var extensions []extension
	var funcs []extension

	for _, f := range files {
		unit := f.Unit
		for _, s := range f.Stmts {
			ds, ok := s.(*ast.DeclStmt)
			if !ok {
				continue
			}
			switch d := ds.D.(type) {
			case *ast.PackageDecl:
				if pkg == "" && d.Name != nil {
					pkg = d.Name.Name(unit)
				}
			case *ast.StructDecl, *ast.EnumDecl, *ast.ClassDecl:
				visitType(unit, d, "")
			case *ast.ExtensionDecl:
				extensions = append(extensions, extension{unit, d})
			case *ast.FuncDecl:
				funcs = append(funcs, extension{unit, &ast.ExtensionDecl{Body: &ast.MemberBlock{Members: []ast.Node{d}}}})
			}
		}
	}
	// Extensions add conformances and functions to a type declared
	// anywhere in the module, so they are read once every type is known.
	for _, e := range extensions {
		path := typePath(e.unit, e.d.Type)
		t := found[path]
		if t == nil {
			continue
		}
		eq, hash := says(e.unit, e.d.Inherit)
		t.equatable, t.hashable = t.equatable || eq, t.hashable || hash
		if e.d.Body == nil {
			continue
		}
		for _, m := range e.d.Body.Members {
			fn, ok := m.(*ast.FuncDecl)
			if !ok {
				continue
			}
			if declaresEquals(e.unit, fn, path) {
				t.hasEquals = true
			}
			if declaresHash(e.unit, fn) {
				t.hasHash = true
			}
		}
	}
	// A `==` at the top level taking the type is its own too.
	for _, e := range funcs {
		fn := e.d.Body.Members[0].(*ast.FuncDecl)
		if fn.Name == nil || fn.Name.Name(e.unit) != "==" || fn.Sig == nil || len(fn.Sig.Params) != 2 {
			continue
		}
		if t := found[typePath(e.unit, fn.Sig.Params[0].Type)]; t != nil {
			t.hasEquals = true
		}
	}

	var b strings.Builder
	for _, path := range order {
		t := found[path]
		// A generic type's == and hash need its parameters to conform too,
		// which a conditional extension says and this does not yet.
		if t.generic {
			continue
		}
		access := ""
		if t.public {
			access = "public "
		}
		// An enum whose cases carry nothing is Equatable and Hashable
		// whether or not it says so, and its hash is its case.
		implicit := ""
		if t.isEnum && !t.payload && len(t.cases) > 0 && !t.hashable {
			implicit = ": Hashable"
			t.hashable = true
		}
		var body strings.Builder
		if !t.isEnum && t.equatable && !t.hasEquals {
			fmt.Fprintf(&body, "    %sstatic func %s(_ a: %s, _ b: %s) -> Bool {\n", access, EqualsName, path, path)
			for _, f := range t.fields {
				fmt.Fprintf(&body, "        if !(a.%s == b.%s) { return false }\n", f, f)
			}
			body.WriteString("        return true\n    }\n")
		}
		// An enum whose cases carry values compares case by case: the same
		// case, and each value equal to the one beside it.
		if t.isEnum && t.payload && t.equatable && !t.hasEquals {
			fmt.Fprintf(&body, "    %sstatic func %s(_ a: %s, _ b: %s) -> Bool {\n", access, EnumEqualsName, path, path)
			body.WriteString("        switch a {\n")
			for _, c := range t.cases {
				if c.arity == 0 {
					fmt.Fprintf(&body, "        case .%s:\n            if case .%s = b { return true }\n            return false\n", c.name, c.name)
					continue
				}
				fmt.Fprintf(&body, "        case .%s(%s):\n            if case .%s(%s) = b { return %s }\n            return false\n",
					c.name, bindings("l", c.arity), c.name, bindings("r", c.arity), pairsEqual(c.arity))
			}
			body.WriteString("        }\n    }\n")
		}
		if t.hashable && !t.hasHash {
			fmt.Fprintf(&body, "    %sfunc hash(into hasher: inout Hasher) {\n", access)
			if t.isEnum && len(t.cases) > 0 {
				body.WriteString("        switch self {\n")
				for i, c := range t.cases {
					if c.arity == 0 {
						fmt.Fprintf(&body, "        case .%s: hasher.combine(%d)\n", c.name, i)
						continue
					}
					fmt.Fprintf(&body, "        case .%s(%s):\n            hasher.combine(%d)\n", c.name, bindings("v", c.arity), i)
					for k := 0; k < c.arity; k++ {
						fmt.Fprintf(&body, "            hasher.combine(v%d)\n", k)
					}
				}
				body.WriteString("        }\n")
			}
			for _, f := range t.fields {
				fmt.Fprintf(&body, "        hasher.combine(self.%s)\n", f)
			}
			body.WriteString("    }\n")
		}
		if body.Len() == 0 {
			continue
		}
		fmt.Fprintf(&b, "extension %s%s {\n%s}\n", path, implicit, body.String())
	}
	if b.Len() == 0 {
		return nil
	}
	head := ""
	if pkg != "" {
		head = "package " + pkg + "\n\n"
	}
	return []byte(head + b.String())
}

// bindings is `let p0, let p1, ...` for a case's values.
func bindings(prefix string, n int) string {
	parts := make([]string, n)
	for i := range parts {
		parts[i] = fmt.Sprintf("let %s%d", prefix, i)
	}
	return strings.Join(parts, ", ")
}

// pairsEqual is `l0 == r0 && l1 == r1 ...`.
func pairsEqual(n int) string {
	parts := make([]string, n)
	for i := range parts {
		parts[i] = fmt.Sprintf("l%d == r%d", i, i)
	}
	return strings.Join(parts, " && ")
}

func join(outer, name string) string {
	if outer == "" {
		return name
	}
	return outer + "." + name
}

// says reports whether an inheritance clause names a protocol that makes a
// type Equatable, and whether one that makes it Hashable.
func says(unit *token.File, in *ast.InheritanceClause) (equatable, hashable bool) {
	if in == nil {
		return false, false
	}
	for _, item := range in.Items {
		name := typePath(unit, item.Type)
		if i := strings.LastIndex(name, "."); i >= 0 && name[:i] == "Swift" {
			name = name[i+1:]
		}
		equatable = equatable || equatables[name]
		hashable = hashable || name == "Hashable"
	}
	return equatable, hashable
}

// declaresHash reports whether a function is a hash(into:).
func declaresHash(unit *token.File, fn *ast.FuncDecl) bool {
	if fn.Name == nil || fn.Name.Name(unit) != "hash" || fn.Sig == nil || len(fn.Sig.Params) != 1 {
		return false
	}
	p := fn.Sig.Params[0]
	return p.Label != nil && p.Label.Name(unit) == "into"
}

// typePath is a type written as a name, `Outer.Inner`, or "".
func typePath(unit *token.File, t ast.Type) string {
	switch x := t.(type) {
	case *ast.IdentType:
		if x.Name != nil {
			return x.Name.Name(unit)
		}
	case *ast.MemberType:
		if base := typePath(unit, x.X); base != "" && x.Name != nil {
			return base + "." + x.Name.Name(unit)
		}
	}
	return ""
}

// declaresEquals reports whether a function is a `==` of the type, or the
// derived one already -- read back from an interface, say.
func declaresEquals(unit *token.File, fn *ast.FuncDecl, path string) bool {
	if fn.Name == nil {
		return false
	}
	switch fn.Name.Name(unit) {
	case EqualsName, EnumEqualsName:
		return true
	case "==":
		if fn.Sig == nil || len(fn.Sig.Params) != 2 {
			return false
		}
		want := path
		if i := strings.LastIndex(path, "."); i >= 0 {
			want = path[i+1:]
		}
		got := typePath(unit, fn.Sig.Params[0].Type)
		_, self := fn.Sig.Params[0].Type.(*ast.SelfType)
		return self || got == path || got == want
	}
	return false
}

// stored is the name a binding stores a property under, and whether it
// stores one: not a computed property, whose accessors get and set.
func stored(unit *token.File, b *ast.PatternBinding) (string, bool) {
	if b.Body != nil {
		return "", false
	}
	if b.Accessors != nil {
		for _, a := range b.Accessors.Accessors {
			if a.Keyword == nil {
				continue
			}
			switch a.Keyword.Name(unit) {
			case "get", "set":
				return "", false
			}
		}
	}
	pat := b.Pat
	if tp, ok := pat.(*ast.TypedPattern); ok {
		pat = tp.Pat
	}
	id, ok := pat.(*ast.IdentPattern)
	if !ok || id.Name == nil {
		return "", false
	}
	return id.Name.Name(unit), true
}

func hasModifier(unit *token.File, mods []*ast.Modifier, word string) bool {
	for _, m := range mods {
		if m == nil {
			continue
		}
		if m.Name != nil && m.Name.Name(unit) == word {
			return true
		}
		if unit != nil && string(unit.Slice(m.Pos(), m.End())) == word {
			return true
		}
	}
	return false
}
