package analyzer

import (
	"fmt"
	"strings"

	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/parser"
	"github.com/vertex-language/vsc/token"
	"github.com/vertex-language/vsc/types"
)

// Derived conformances, as swiftc's Sema makes them (DerivedConformance,
// DerivedConformanceEquatableHashable, DerivedConformanceCaseIterable).
//
// A struct or enum that conforms to Equatable -- itself, or through a
// protocol that refines it: Comparable, Hashable, OptionSet, one of the
// program's own -- and has no `==` of its own gets one, comparing every
// stored property in the order they are declared, or an enum's cases and
// what they carry. One that is Hashable and has no hash(into:) gets that,
// combining the same; so does an enum of cases alone, which is Hashable
// without saying so. An enum of cases alone that is CaseIterable gets its
// allCases. swiftc names the `==` it makes __derived_struct_equals and
// __derived_enum_equals, and so does this.
//
// It is decided here, once every conformance a type has -- from its
// declaration and its extensions -- is known and before any is checked
// for its witnesses, which is where swiftc derives one. The members are
// written as an extension of the type in a file of their own, which is
// checked and lowered with the module like any other: Info.Derived.

// DerivedStructEquals is the name of the `==` swiftc derives for a struct.
const DerivedStructEquals = "__derived_struct_equals"

// DerivedEnumEquals is the name of the `==` swiftc derives for an enum whose
// cases carry values. One of cases alone compares its tags, and needs none.
const DerivedEnumEquals = "__derived_enum_equals"

// derivable is a struct or enum of the module, with what deriving asks of
// its declaration.
type derivable struct {
	path    string // Outer.Inner
	typ     types.Type
	isEnum  bool
	public  bool
	params  []string   // a generic type's parameters
	generic bool       // it has parameters, named or not
	fields  []string   // a struct's stored properties
	cases   []enumCase // an enum's cases
	payload bool       // an enum case carries a value
}

// enumCase is one case of an enum: its name, and how many values it
// carries.
type enumCase struct {
	name  string
	arity int
}

// deriveConformances writes the members the module's types get by
// conforming, checks them as members of those types, and answers the
// file they are in, or nil where there are none.
func (c *checker) deriveConformances(files []*ast.File, scope *Scope) *ast.File {
	equatable, _ := c.coreProtocol("Equatable")
	hashable, _ := c.coreProtocol("Hashable")
	caseIterable, _ := c.coreProtocol("CaseIterable")
	if equatable == nil || hashable == nil {
		return nil
	}

	var found []*derivable
	pkg := ""
	for _, f := range files {
		c.file = f.Unit
		for _, s := range f.Stmts {
			ds, ok := s.(*ast.DeclStmt)
			if !ok {
				continue
			}
			if p, ok := ds.D.(*ast.PackageDecl); ok && pkg == "" && p.Name != nil {
				pkg = p.Name.Text(c.file)
			}
			found = c.derivables(ds.D, "", found)
		}
	}

	var b strings.Builder
	for _, t := range found {
		var body strings.Builder
		conformance := ""
		isEquatable := c.conformsTo(t.typ, equatable)
		isHashable := c.conformsTo(t.typ, hashable)
		// An enum of cases alone is Equatable and Hashable whether or not
		// it says so; where it does not, the derived extension does.
		if t.isEnum && !t.payload && len(t.cases) > 0 && !declares(t.typ, hashable) {
			conformance = ": Hashable"
			isEquatable, isHashable = true, true
		}
		// A generic type's members need its parameters to conform too: they
		// are in an extension that says so, so that Pair<T> is Equatable
		// where T is -- and hashes where T is Hashable.
		where := ""
		if t.generic {
			if len(t.params) == 0 || !isEquatable {
				continue
			}
			want := "Equatable"
			if isHashable && !c.hasHash(t.typ) {
				want = "Hashable"
			}
			parts := make([]string, len(t.params))
			for i, p := range t.params {
				parts[i] = p + ": " + want
			}
			where = " where " + strings.Join(parts, ", ")
		}
		if isEquatable && !c.hasEquals(t.typ, scope) {
			c.deriveEquatable(&body, t)
		}
		if isHashable && !c.hasHash(t.typ) {
			c.deriveHashable(&body, t)
		}
		if caseIterable != nil && t.isEnum && !t.payload && c.conformsTo(t.typ, caseIterable) && !c.hasStatic(t.typ, "allCases") {
			c.deriveCaseIterable(&body, t)
		}
		if body.Len() == 0 {
			continue
		}
		fmt.Fprintf(&b, "extension %s%s%s {\n%s}\n", t.path, conformance, where, body.String())
	}
	if b.Len() == 0 {
		return nil
	}
	head := ""
	if pkg != "" {
		head = "package " + pkg + "\n\n"
	}
	unit := token.NewFile("<derived conformances>", []byte(head+b.String()))
	file, ds := parser.ParseFile(unit, 0)
	if len(ds) > 0 {
		return nil
	}
	// The extension's members join their types', as a written one's do.
	c.file = unit
	decls := declsOf(file.Stmts)
	c.resolveExtensions(decls, scope)
	c.resolveAssociatedTypes(decls, scope)
	return file
}

// derivables adds the structs and enums a declaration declares -- it, and
// those nested in it -- to found.
func (c *checker) derivables(d ast.Decl, outer string, found []*derivable) []*derivable {
	var name *ast.Ident
	var body *ast.MemberBlock
	var mods []*ast.Modifier
	var generics *ast.GenericParams
	isEnum := false
	switch x := d.(type) {
	case *ast.StructDecl:
		name, body, mods, generics = x.Name, x.Body, x.Mods, x.Generics
	case *ast.EnumDecl:
		name, body, mods, generics, isEnum = x.Name, x.Body, x.Mods, x.Generics, true
	case *ast.ClassDecl:
		// A class derives nothing, but a type nested in one may.
		if x.Name != nil && x.Body != nil {
			path := join(outer, x.Name.Text(c.file))
			for _, m := range x.Body.Members {
				if md, ok := m.(ast.Decl); ok {
					found = c.derivables(md, path, found)
				}
			}
		}
		return found
	default:
		return found
	}
	if name == nil {
		return found
	}
	sym, _ := c.info.Defs[name].(*TypeNameSymbol)
	if sym == nil || sym.Type() == nil {
		return found
	}
	t := &derivable{
		path:   join(outer, name.Text(c.file)),
		typ:    sym.Type(),
		isEnum: isEnum,
		public: c.hasModifier(mods, "public") || c.hasModifier(mods, "open"),
	}
	if generics != nil {
		t.generic = true
		for _, gp := range generics.Params {
			if gp.Name == nil || gp.Each.IsValid() || gp.Let.IsValid() {
				t.params = nil
				break
			}
			t.params = append(t.params, gp.Name.Text(c.file))
		}
	}
	found = append(found, t)
	if body == nil {
		return found
	}
	for _, m := range body.Members {
		switch x := m.(type) {
		case *ast.EnumCaseDecl:
			for _, el := range x.Elements {
				if el.Name != nil {
					t.cases = append(t.cases, enumCase{name: el.Name.Text(c.file), arity: len(el.Params)})
				}
				if el.Lparen.IsValid() {
					t.payload = true
				}
			}
		case *ast.VarDecl:
			if isEnum || isStatic(x.Mods) {
				continue
			}
			for _, b := range x.Bindings {
				if name, ok := c.storedName(b); ok {
					t.fields = append(t.fields, name)
				}
			}
		case *ast.StructDecl, *ast.EnumDecl, *ast.ClassDecl:
			found = c.derivables(x.(ast.Decl), t.path, found)
		}
	}
	return found
}

// deriveEquatable writes the `==` of a type that has none: a struct's
// compares its stored properties, an enum's its cases and what they carry.
// An enum of cases alone compares its tags and needs none.
func (c *checker) deriveEquatable(body *strings.Builder, t *derivable) {
	access := accessOf(t)
	self := selfOf(t)
	if !t.isEnum {
		fmt.Fprintf(body, "    %sstatic func %s(_ a: %s, _ b: %s) -> Bool {\n", access, DerivedStructEquals, self, self)
		for _, f := range t.fields {
			fmt.Fprintf(body, "        if !(a.%s == b.%s) { return false }\n", f, f)
		}
		body.WriteString("        return true\n    }\n")
		return
	}
	if !t.payload {
		return
	}
	fmt.Fprintf(body, "    %sstatic func %s(_ a: %s, _ b: %s) -> Bool {\n", access, DerivedEnumEquals, self, self)
	body.WriteString("        switch a {\n")
	for _, k := range t.cases {
		if k.arity == 0 {
			fmt.Fprintf(body, "        case .%s:\n            if case .%s = b { return true }\n            return false\n", k.name, k.name)
			continue
		}
		fmt.Fprintf(body, "        case .%s(%s):\n            if case .%s(%s) = b { return %s }\n            return false\n",
			k.name, bindings("l", k.arity), k.name, bindings("r", k.arity), pairsEqual(k.arity))
	}
	body.WriteString("        }\n    }\n")
}

// deriveHashable writes the hash(into:) of a type that has none: a
// struct's combines its stored properties, an enum's its case and what it
// carries.
func (c *checker) deriveHashable(body *strings.Builder, t *derivable) {
	fmt.Fprintf(body, "    %sfunc hash(into hasher: inout Hasher) {\n", accessOf(t))
	if t.isEnum && len(t.cases) > 0 {
		body.WriteString("        switch self {\n")
		for i, k := range t.cases {
			if k.arity == 0 {
				fmt.Fprintf(body, "        case .%s: hasher.combine(%d)\n", k.name, i)
				continue
			}
			fmt.Fprintf(body, "        case .%s(%s):\n            hasher.combine(%d)\n", k.name, bindings("v", k.arity), i)
			for n := 0; n < k.arity; n++ {
				fmt.Fprintf(body, "            hasher.combine(v%d)\n", n)
			}
		}
		body.WriteString("        }\n")
	}
	for _, f := range t.fields {
		fmt.Fprintf(body, "        hasher.combine(self.%s)\n", f)
	}
	body.WriteString("    }\n")
}

// deriveCaseIterable writes the allCases of an enum of cases alone: its
// cases, in the order they are declared.
func (c *checker) deriveCaseIterable(body *strings.Builder, t *derivable) {
	names := make([]string, len(t.cases))
	for i, k := range t.cases {
		names[i] = "." + k.name
	}
	fmt.Fprintf(body, "    %sstatic var allCases: [%s] { [%s] }\n", accessOf(t), t.path, strings.Join(names, ", "))
}

// hasEquals reports whether t has a `==` of its own -- a static one it or
// an extension declares, one at the top of the module taking it, or the
// derived one, read back from an interface.
func (c *checker) hasEquals(t types.Type, scope *Scope) bool {
	for _, m := range methodsOf(t) {
		if !m.IsStatic || m.Sig == nil {
			continue
		}
		switch m.Name {
		case DerivedStructEquals, DerivedEnumEquals:
			return true
		case "==":
			if len(m.Sig.Params) == 2 && types.Identical(m.Sig.Params[0].Type, t) {
				return true
			}
		}
	}
	if fn, ok := scope.LookupLocal("==").(*FuncSymbol); ok {
		for _, o := range fn.Overloads() {
			sig := o.Signature()
			if sig != nil && len(sig.Params) == 2 && types.Identical(sig.Params[0].Type, t) {
				return true
			}
		}
	}
	return false
}

// hasHash reports whether t has a hash(into:) of its own.
func (c *checker) hasHash(t types.Type) bool {
	for _, m := range methodsOf(t) {
		if m.Name == "hash" && !m.IsStatic && m.Sig != nil && len(m.Sig.Params) == 1 && m.Sig.Params[0].Label == "into" {
			return true
		}
	}
	return false
}

// hasStatic reports whether t has a static property of the name.
func (c *checker) hasStatic(t types.Type, name string) bool {
	var statics []*types.Field
	switch u := t.Underlying().(type) {
	case *types.Struct:
		statics = u.Statics
	case *types.Enum:
		statics = u.Statics
	}
	for _, f := range statics {
		if f != nil && f.Name == name {
			return true
		}
	}
	return false
}

// methodsOf is the methods a struct or enum, and its extensions, declare.
func methodsOf(t types.Type) []*types.Method {
	switch u := t.Underlying().(type) {
	case *types.Struct:
		return u.Methods
	case *types.Enum:
		return u.Methods
	}
	return nil
}

// storedName is the name a binding stores a property under, and whether it
// stores one: not a computed property, whose accessors get and set.
func (c *checker) storedName(b *ast.PatternBinding) (string, bool) {
	if b.Body != nil {
		return "", false
	}
	if b.Accessors != nil {
		for _, a := range b.Accessors.Accessors {
			if a.Keyword == nil {
				continue
			}
			switch a.Keyword.Text(c.file) {
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
	return id.Name.Text(c.file), true
}

// accessOf is how a derived member of t is declared: public where t is.
func accessOf(t *derivable) string {
	if t.public {
		return "public "
	}
	return ""
}

// selfOf is t as its members name it: Pair<T>, for a generic one.
func selfOf(t *derivable) string {
	if len(t.params) > 0 {
		return t.path + "<" + strings.Join(t.params, ", ") + ">"
	}
	return t.path
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

// declares reports whether t says it conforms to p, or to a protocol that
// refines p: in its declaration or an extension, not implicitly.
func declares(t types.Type, p *types.Protocol) bool {
	var list []*types.Protocol
	switch u := t.Underlying().(type) {
	case *types.Struct:
		list = u.Conformances
	case *types.Enum:
		list = u.Conformances
	}
	for _, q := range list {
		if types.Identical(q, p) || types.ConformsTo(q, p) {
			return true
		}
	}
	return false
}
