// Package doc extracts a Vertex package's documentation: its exported
// declarations, as written, with the comments above them. It is go/doc
// for Vertex, and what `vsc doc` renders as Markdown or HTML.
//
// Nothing here type-checks. A package is its files parsed, and what it
// exports is what is marked public or open -- every member of a public
// protocol, and every case of a public enum, as Swift has it -- so a
// package that does not compile still has its documentation read.
//
// A declaration's documentation is the run of comments that ends on the
// line above it: /// lines as Swift writes them, or // lines as Go does.
// The package's own is the comment above its package clause in any of
// its files, which by convention begins "Package name".
package doc

import (
	"sort"
	"strings"

	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/token"
)

// A Package is what a package documents.
type Package struct {
	// Name is the package clause's name, or the folder's.
	Name string
	// ImportPath is the path a program imports it by, "" where unknown.
	ImportPath string
	// Doc is the package comment, with the comment markers taken off.
	Doc string
	// Consts and Vars are the top-level lets and vars.
	Consts []*Decl
	Vars   []*Decl
	// Funcs are the top-level functions no type's receiver claims.
	Funcs []*Decl
	// Types are the types declared, with what extends them here, by name.
	Types []*Type
	// Extensions are extensions here of types declared elsewhere.
	Extensions []*Type
	// Other is what is exported and fits none of the above: operators,
	// precedence groups, macros.
	Other []*Decl
	// Files are the files read, by base name.
	Files []string
}

// A Decl is one declaration: how it is written, without its body, and the
// comment above it.
type Decl struct {
	// Name is what the declaration declares: "Seconds", "init", "+".
	Name string
	// Kind is the keyword that declares it: "func", "struct", "let"...
	Kind string
	// Sig is the declaration as written, attributes to where its body
	// begins, dedented.
	Sig string
	Doc string
	// File and Line are where it is written.
	File string
	Line int
	// ID is its fragment identifier in a rendered page, unique in it:
	// "func-Sleep", "Duration.Seconds", "Duration.Seconds-2" for an
	// overload.
	ID string
}

// A Type is a type declaration with its members: those in its body, those
// its extensions here add, and the functions whose receiver is it.
type Type struct {
	*Decl
	// Conforms are the conformances its extensions add, as written.
	Conforms []string
	// Cases are an enum's cases.
	Cases []*Decl
	// Inits are its initializers.
	Inits []*Decl
	// Members are its properties, methods, subscripts and nested types,
	// in written order, extensions' after the body's.
	Members []*Decl
}

// Options says what New reads.
type Options struct {
	// All documents every declaration, not only the exported ones, as
	// `go doc -u` does.
	All bool
	// ImportPath is recorded as the package's.
	ImportPath string
}

// New reads the documentation of the package whose files these are. files
// and units are parallel, and the files must have been parsed with
// parser.ParseComments for any comment to be found.
func New(files []*ast.File, units []*token.File, opts Options) *Package {
	r := &reader{opts: opts, types: map[string]*Type{}, pkg: &Package{ImportPath: opts.ImportPath}}
	for i, f := range files {
		unit := f.Unit
		if unit == nil && i < len(units) {
			unit = units[i]
		}
		if unit == nil {
			continue
		}
		r.file = unit
		r.comments = f.Comments
		r.pkg.Files = append(r.pkg.Files, baseName(unit.Name()))
		r.stmts(f.Stmts)
	}
	r.attach()
	r.sort()
	r.pkg.assignIDs()
	return r.pkg
}

type reader struct {
	opts     Options
	pkg      *Package
	file     *token.File
	comments []token.Token
	// types are the package's types by name, declared or extended.
	types map[string]*Type
	// order is the types in the order they were first seen.
	order []string
	// extended are extensions and receiver functions of types, by name,
	// held until every file is read and it is known which are local.
	extended []pending
	// seen keeps one of each signature, for declarations written in
	// several #if clauses.
	seen map[string]bool
}

// pending is a member for a type whose declaration may not have been
// read yet.
type pending struct {
	typ      string
	conforms string
	decl     *Decl
	ext      *Decl // the extension's own declaration, for a foreign type
}

func (r *reader) stmts(stmts []ast.Stmt) {
	for _, s := range stmts {
		switch s := s.(type) {
		case *ast.DeclStmt:
			r.decl(s.D)
		case *ast.IfConfigStmt:
			for _, c := range s.Clauses {
				r.stmts(c.Stmts)
			}
		}
	}
}

func (r *reader) decl(d ast.Decl) {
	switch d := d.(type) {
	case *ast.PackageDecl:
		if r.pkg.Name == "" && d.Name != nil {
			r.pkg.Name = d.Name.Text(r.file)
		}
		if doc := r.docFor(d.Pos()); doc != "" && (r.pkg.Doc == "" || strings.HasPrefix(doc, "Package ")) {
			r.pkg.Doc = doc
		}
	case *ast.VarDecl:
		if !r.exported(d.Mods, false) {
			return
		}
		decl := r.varDecl(d)
		if decl == nil {
			return
		}
		if decl.Kind == "let" {
			r.pkg.Consts = append(r.pkg.Consts, decl)
		} else {
			r.pkg.Vars = append(r.pkg.Vars, decl)
		}
	case *ast.FuncDecl:
		if !r.exported(d.Mods, false) {
			return
		}
		decl := r.make(d, "func", r.ident(d.Name), bodyPos(d.Body))
		if d.Recv != nil {
			r.extended = append(r.extended, pending{typ: typeName(r.text(d.Recv.Type)), decl: decl})
			return
		}
		r.add(&r.pkg.Funcs, decl)
	case *ast.ExtensionDecl:
		name := typeName(r.text(d.Type))
		public := hasAccess(r.file, d.Mods, "public", "open")
		ext := r.make(d, "extension", r.text(d.Type), bodyPosM(d.Body))
		conforms := ""
		if d.Inherit != nil {
			conforms = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(r.text(d.Inherit)), ":"))
		}
		members := r.members(d.Body, public)
		if len(members) == 0 && conforms == "" {
			return
		}
		if conforms != "" {
			r.extended = append(r.extended, pending{typ: name, conforms: conforms, ext: ext})
		}
		for _, m := range members {
			r.extended = append(r.extended, pending{typ: name, decl: m, ext: ext})
		}
	case *ast.StructDecl:
		r.typeDecl(d, d.Mods, "struct", d.Name, d.Body, false, "")
	case *ast.ClassDecl:
		r.typeDecl(d, d.Mods, "class", d.Name, d.Body, false, "")
	case *ast.ActorDecl:
		r.typeDecl(d, d.Mods, "actor", d.Name, d.Body, false, "")
	case *ast.EnumDecl:
		r.typeDecl(d, d.Mods, "enum", d.Name, d.Body, false, "")
	case *ast.ProtocolDecl:
		r.typeDecl(d, d.Mods, "protocol", d.Name, d.Body, true, "")
	case *ast.TypealiasDecl:
		if !r.exported(d.Mods, false) {
			return
		}
		r.typeDecl(d, d.Mods, "typealias", d.Name, nil, false, "")
	case *ast.OperatorDecl, *ast.PrecedenceGroupDecl:
		// Operators have no access level: they are the module's to use,
		// and every client's once a public function implements one.
		name := ""
		switch d := d.(type) {
		case *ast.OperatorDecl:
			name = r.ident(d.Name)
		case *ast.PrecedenceGroupDecl:
			name = r.ident(d.Name)
		}
		r.add(&r.pkg.Other, r.make(d, kindOf(d), name, token.NoPos))
	case *ast.MacroDecl:
		if r.exported(d.Mods, false) {
			r.add(&r.pkg.Other, r.make(d, "macro", r.ident(d.Name), token.NoPos))
		}
	}
}

// typeDecl records a type declaration and its members. prefix is the
// enclosing type's name and a dot, for a nested type.
func (r *reader) typeDecl(d ast.Decl, mods []*ast.Modifier, kind string, name *ast.Ident, body *ast.MemberBlock, protocol bool, prefix string) {
	if !r.exported(mods, false) {
		return
	}
	full := prefix + r.ident(name)
	var brace token.Pos
	if body != nil {
		brace = body.Lbrace
	}
	t := r.typ(full)
	if t.Decl != nil {
		return // one of several #if clauses
	}
	t.Decl = r.make(d, kind, full, brace)
	if body == nil {
		return
	}
	public := protocol
	for _, m := range r.memberNodes(body.Members) {
		switch m := m.(type) {
		case *ast.EnumCaseDecl:
			for _, e := range m.Elements {
				c := r.make(m, "case", r.ident(e.Name), token.NoPos)
				if len(m.Elements) > 1 {
					c.Sig = "case " + strings.TrimSpace(r.text(e))
				}
				t.Cases = append(t.Cases, c)
			}
			continue
		case *ast.StructDecl:
			r.typeDecl(m, m.Mods, "struct", m.Name, m.Body, false, full+".")
			continue
		case *ast.ClassDecl:
			r.typeDecl(m, m.Mods, "class", m.Name, m.Body, false, full+".")
			continue
		case *ast.ActorDecl:
			r.typeDecl(m, m.Mods, "actor", m.Name, m.Body, false, full+".")
			continue
		case *ast.EnumDecl:
			r.typeDecl(m, m.Mods, "enum", m.Name, m.Body, false, full+".")
			continue
		case *ast.ProtocolDecl:
			r.typeDecl(m, m.Mods, "protocol", m.Name, m.Body, true, full+".")
			continue
		}
		if decl := r.member(m, public); decl != nil {
			if decl.Kind == "init" {
				t.Inits = append(t.Inits, decl)
			} else {
				t.Members = append(t.Members, decl)
			}
		}
	}
}

// members are the documented members of an extension's body. public says
// the extension is, which makes its members so unless they say otherwise.
func (r *reader) members(body *ast.MemberBlock, public bool) []*Decl {
	if body == nil {
		return nil
	}
	var out []*Decl
	for _, m := range r.memberNodes(body.Members) {
		if decl := r.member(m, public); decl != nil {
			out = append(out, decl)
		}
	}
	return out
}

// memberNodes flattens the #if clauses among members.
func (r *reader) memberNodes(nodes []ast.Node) []ast.Node {
	var out []ast.Node
	for _, n := range nodes {
		switch n := n.(type) {
		case *ast.IfConfigStmt:
			for _, c := range n.Clauses {
				var inner []ast.Node
				for _, s := range c.Stmts {
					if ds, ok := s.(*ast.DeclStmt); ok {
						inner = append(inner, ds.D)
					} else {
						inner = append(inner, s)
					}
				}
				out = append(out, r.memberNodes(inner)...)
			}
		case *ast.DeclStmt:
			out = append(out, n.D)
		default:
			out = append(out, n)
		}
	}
	return out
}

// member is one member's documentation, or nil where it is not exported.
func (r *reader) member(n ast.Node, public bool) *Decl {
	switch m := n.(type) {
	case *ast.VarDecl:
		if r.exported(m.Mods, public) {
			return r.varDecl(m)
		}
	case *ast.FuncDecl:
		if r.exported(m.Mods, public) {
			return r.make(m, "func", r.ident(m.Name), bodyPos(m.Body))
		}
	case *ast.InitDecl:
		if r.exported(m.Mods, public) {
			return r.make(m, "init", "init", bodyPos(m.Body))
		}
	case *ast.SubscriptDecl:
		if r.exported(m.Mods, public) {
			d := r.make(m, "subscript", "subscript", r.accessorsPos(m.Body, m.Accessors))
			d.Sig += accessorSummary(r.file, m.Body, m.Accessors, m.Mods)
			return d
		}
	case *ast.TypealiasDecl:
		if r.exported(m.Mods, public) {
			return r.make(m, "typealias", r.ident(m.Name), token.NoPos)
		}
	case *ast.AssociatedTypeDecl:
		return r.make(m, "associatedtype", r.ident(m.Name), token.NoPos)
	}
	return nil
}

// varDecl documents a let or var: its bindings as written, a computed
// property's body summarized as { get } or { get set }, and an initializer
// kept only where it is short enough to read at a glance.
func (r *reader) varDecl(d *ast.VarDecl) *Decl {
	kind := "var"
	if d.Kind == token.LET {
		kind = "let"
	}
	name := ""
	if len(d.Bindings) > 0 {
		name = patternName(r.text(d.Bindings[0].Pat))
	}
	decl := r.make(d, kind, name, token.NoPos)
	if len(d.Bindings) == 1 {
		b := d.Bindings[0]
		switch {
		case b.Body != nil || b.Accessors != nil:
			decl.Sig = dedent(strings.TrimRight(r.file.String(r.start(d), r.accessorsPos(b.Body, b.Accessors)), " \t\r\n"), r.column(d)) +
				accessorSummary(r.file, b.Body, b.Accessors, d.Mods)
		case b.Assign.IsValid() && b.Value != nil:
			value := strings.TrimSpace(r.text(b.Value))
			decl.Sig = dedent(strings.TrimRight(r.file.String(r.start(d), b.Assign), " \t"), r.column(d))
			if len(value) <= 60 && !strings.Contains(value, "\n") {
				decl.Sig += " = " + value
			}
		}
	}
	return decl
}

// make documents a declaration written from its start to end: the body's
// brace, or NoPos for the whole of it.
func (r *reader) make(n ast.Node, kind, name string, end token.Pos) *Decl {
	start := r.start(n)
	if !end.IsValid() {
		end = n.End()
	}
	sig := strings.TrimRight(r.file.String(start, end), " \t\r\n")
	return &Decl{
		Name: name,
		Kind: kind,
		Sig:  dedent(sig, r.column(n)),
		Doc:  r.docFor(start),
		File: baseName(r.file.Name()),
		Line: r.file.Line(start),
	}
}

func (r *reader) start(n ast.Node) token.Pos { return n.Pos() }

func (r *reader) column(n ast.Node) int { return r.file.Position(r.start(n)).Column }

func (r *reader) add(list *[]*Decl, d *Decl) {
	if r.seen == nil {
		r.seen = map[string]bool{}
	}
	if r.seen[d.Kind+"\x00"+d.Sig] {
		return
	}
	r.seen[d.Kind+"\x00"+d.Sig] = true
	*list = append(*list, d)
}

func (r *reader) typ(name string) *Type {
	t, ok := r.types[name]
	if !ok {
		t = &Type{}
		r.types[name] = t
		r.order = append(r.order, name)
	}
	return t
}

// attach gives each extension and receiver function to its type: the
// package's own where it declares one of the name, and an entry under
// Extensions where it does not.
func (r *reader) attach() {
	foreign := map[string]*Type{}
	var foreignOrder []string
	for _, p := range r.extended {
		t, ok := r.types[p.typ]
		if !ok || t.Decl == nil {
			t, ok = foreign[p.typ]
			if !ok {
				t = &Type{Decl: &Decl{Name: p.typ, Kind: "extension", Sig: "extension " + p.typ}}
				if p.ext != nil {
					t.Decl.File, t.Decl.Line = p.ext.File, p.ext.Line
				}
				foreign[p.typ] = t
				foreignOrder = append(foreignOrder, p.typ)
			}
		}
		if p.conforms != "" {
			t.Conforms = append(t.Conforms, p.conforms)
		}
		if p.decl == nil {
			continue
		}
		if p.decl.Kind == "init" {
			t.Inits = append(t.Inits, p.decl)
		} else {
			t.Members = append(t.Members, p.decl)
		}
	}
	for _, name := range r.order {
		if t := r.types[name]; t.Decl != nil {
			r.pkg.Types = append(r.pkg.Types, t)
		}
	}
	for _, name := range foreignOrder {
		r.pkg.Extensions = append(r.pkg.Extensions, foreign[name])
	}
}

// sort orders what a reader looks things up in by name, as godoc does;
// a type's members stay in the order they are written.
func (r *reader) sort() {
	byName := func(list []*Decl) {
		sort.SliceStable(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	}
	byName(r.pkg.Consts)
	byName(r.pkg.Vars)
	byName(r.pkg.Funcs)
	sort.SliceStable(r.pkg.Types, func(i, j int) bool { return r.pkg.Types[i].Name < r.pkg.Types[j].Name })
	sort.SliceStable(r.pkg.Extensions, func(i, j int) bool { return r.pkg.Extensions[i].Name < r.pkg.Extensions[j].Name })
}

// exported reports whether a declaration with these modifiers is
// documented: it says public or open, or it says no access level and
// public is its default -- a protocol's requirement, a public
// extension's member.
func (r *reader) exported(mods []*ast.Modifier, public bool) bool {
	if r.opts.All {
		return true
	}
	if hasAccess(r.file, mods, "public", "open") {
		return true
	}
	return public && !hasAccess(r.file, mods, "private", "fileprivate", "internal", "package")
}

// hasAccess reports whether mods hold one of the access levels named,
// setter-only ones -- private(set) -- aside.
func hasAccess(f *token.File, mods []*ast.Modifier, levels ...string) bool {
	for _, m := range mods {
		if m.Arg != nil || m.Name == nil {
			continue
		}
		word := m.Name.Text(f)
		for _, l := range levels {
			if word == l {
				return true
			}
		}
	}
	return false
}

// docFor is the comment that ends on the line above pos, with the lines
// of it that touch joined: the doc comment of whatever starts at pos.
func (r *reader) docFor(pos token.Pos) string {
	cs := r.comments
	i := sort.Search(len(cs), func(i int) bool { return cs[i].End > pos }) - 1
	if i < 0 {
		return ""
	}
	line := r.file.Line(pos)
	if r.file.Line(cs[i].End-1) != line-1 || !r.blank(cs[i].End, pos) {
		return ""
	}
	first := i
	for first > 0 {
		prev := cs[first-1]
		if r.file.Line(prev.End-1) != r.file.Line(cs[first].Pos)-1 || !r.blank(prev.End, cs[first].Pos) {
			break
		}
		first--
	}
	var lines []string
	for _, c := range cs[first : i+1] {
		lines = append(lines, commentLines(r.file.String(c.Pos, c.End))...)
	}
	return strings.Trim(strings.Join(unindent(lines), "\n"), "\n")
}

// blank reports whether nothing but white space lies between two positions.
func (r *reader) blank(from, to token.Pos) bool {
	if from > to {
		return false
	}
	return strings.TrimSpace(r.file.String(from, to)) == ""
}

func (r *reader) ident(id *ast.Ident) string {
	if id == nil {
		return ""
	}
	return id.Text(r.file)
}

func (r *reader) text(n ast.Node) string {
	if n == nil || !n.Pos().IsValid() {
		return ""
	}
	return r.file.String(n.Pos(), n.End())
}

// accessorsPos is where a property's or subscript's body begins.
func (r *reader) accessorsPos(body *ast.CodeBlock, acc *ast.AccessorBlock) token.Pos {
	if acc != nil {
		return acc.Lbrace
	}
	if body != nil {
		return body.Lbrace
	}
	return token.NoPos
}

// accessorSummary is what a computed property or subscript lets a client
// do: " { get }" or " { get set }".
func accessorSummary(f *token.File, body *ast.CodeBlock, acc *ast.AccessorBlock, mods []*ast.Modifier) string {
	if body == nil && acc == nil {
		return ""
	}
	set := false
	if acc != nil {
		for _, a := range acc.Accessors {
			if a.Keyword == nil {
				continue
			}
			switch a.Keyword.Text(f) {
			case "set", "willSet", "didSet", "_modify":
				set = true
			}
		}
		// A stored property with observers has a setter too; one only
		// readable from outside says so with private(set).
		for _, m := range mods {
			if m.Arg != nil && m.Name != nil && m.Arg.Text(f) == "set" && m.Name.Text(f) != "public" && m.Name.Text(f) != "open" {
				set = false
			}
		}
	}
	if set {
		return " { get set }"
	}
	return " { get }"
}

func bodyPos(b *ast.CodeBlock) token.Pos {
	if b == nil {
		return token.NoPos
	}
	return b.Lbrace
}

func bodyPosM(b *ast.MemberBlock) token.Pos {
	if b == nil {
		return token.NoPos
	}
	return b.Lbrace
}

func kindOf(d ast.Decl) string {
	switch d.(type) {
	case *ast.OperatorDecl:
		return "operator"
	case *ast.PrecedenceGroupDecl:
		return "precedencegroup"
	}
	return ""
}

// typeName is the nominal type a written type names: Array<T> is Array,
// Duration? is Duration, a.b.C is a.b.C.
func typeName(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, "<[?!( "); i > 0 {
		s = s[:i]
	}
	return s
}

// patternName is a binding pattern's first name: x in `x: int`.
func patternName(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimLeft(s, "(")
	if i := strings.IndexAny(s, ":,) ="); i > 0 {
		s = s[:i]
	}
	return strings.Trim(s, "`")
}

// commentLines is one comment's text, line by line, without its markers.
func commentLines(c string) []string {
	switch {
	case strings.HasPrefix(c, "/**"), strings.HasPrefix(c, "/*"):
		c = strings.TrimPrefix(c, "/**")
		c = strings.TrimPrefix(c, "/*")
		c = strings.TrimSuffix(c, "*/")
		lines := strings.Split(c, "\n")
		for i, l := range lines {
			t := strings.TrimLeft(l, " \t")
			if strings.HasPrefix(t, "* ") || t == "*" {
				lines[i] = strings.TrimPrefix(strings.TrimPrefix(t, "*"), "")
			}
			lines[i] = strings.TrimRight(lines[i], " \t\r")
		}
		return lines
	case strings.HasPrefix(c, "///"):
		return []string{strings.TrimRight(strings.TrimPrefix(c, "///"), " \t\r")}
	case strings.HasPrefix(c, "//"):
		return []string{strings.TrimRight(strings.TrimPrefix(c, "//"), " \t\r")}
	}
	return []string{c}
}

// unindent takes off the leading space every non-blank line has: the one
// after the marker, and nothing that indents a code block more.
func unindent(lines []string) []string {
	common := -1
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		n := len(l) - len(strings.TrimLeft(l, " "))
		if common < 0 || n < common {
			common = n
		}
	}
	if common <= 0 {
		return lines
	}
	out := make([]string, len(lines))
	for i, l := range lines {
		if len(l) >= common {
			out[i] = l[common:]
		} else {
			out[i] = strings.TrimLeft(l, " ")
		}
	}
	return out
}

// dedent takes the indentation of a declaration's first line, at column
// col, off the lines after it.
func dedent(s string, col int) string {
	if col <= 1 || !strings.Contains(s, "\n") {
		return s
	}
	lines := strings.Split(s, "\n")
	for i := 1; i < len(lines); i++ {
		l := lines[i]
		n := 0
		for n < len(l) && n < col-1 && (l[n] == ' ' || l[n] == '\t') {
			n++
		}
		lines[i] = l[n:]
	}
	return strings.Join(lines, "\n")
}

func baseName(p string) string {
	if i := strings.LastIndexAny(p, `/\`); i >= 0 {
		return p[i+1:]
	}
	return p
}
