package vsc

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"hash"
	"sort"

	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/token"
)

// surface hashes what a client's compile of a package can depend on: the
// package's source with the bodies no client compiles taken out.
//
// A client calls an imported function by its symbol, and compiles only the
// bodies it specializes or inlines: generic functions, every member of a
// generic type or of an extension or receiver of one, and @inlinable
// methods and initializers (see analyzer's checkImportedGenerics). So a
// body outside those can change without any client's object changing,
// and a build that keys a package on its imports' surfaces rather than
// their sources need not rebuild a client for it: early cutoff.
//
// Everything else stays, declarations and initializers and default
// values alike. Where it is not certain that a body is one no client
// compiles -- an extension of a type declared elsewhere, which may be
// generic -- the body stays too: keeping one costs a rebuild, and
// dropping one a client needs would cost a stale object.
//
// Source positions reach generated code only as a line, a column and a
// file name: #line and #column in a body a client compiles, or taken by a
// call there from a default argument (precondition's line: UInt = #line).
// So where such a body, or a #line, #column or #sourceLocation, follows a
// body taken out in the same file, the body taken out is recorded as the
// lines it spans and -- where kept text follows it on its closing line --
// the column it closes at, and every position after it reads the same
// whatever the body held. Where nothing follows that could read a
// position, the body is taken out whole, so that an edit adding lines to
// it is cut off too.
func surface(files []*ast.File, units []*token.File) [32]byte {
	s := newSurfacer(files, units)
	h := sha256.New()
	fmt.Fprintf(h, "surface 1\n%d\n", len(files))
	for i, f := range files {
		if s.file(f, unitOf(f, units, i)) {
			s.write(h)
		}
	}
	var sum [32]byte
	h.Sum(sum[:0])
	return sum
}

// prune is each file's text with the bodies no client compiles blanked:
// every byte of them but line breaks made a space, so that each position
// left reads the same line, column and offset as in the source. It is
// what an importer needs of the files, and parses in a fraction of the
// time. Its surface is the source's.
func prune(files []*ast.File, units []*token.File) [][]byte {
	s := newSurfacer(files, units)
	out := make([][]byte, len(files))
	for i, f := range files {
		unit := unitOf(f, units, i)
		if !s.file(f, unit) {
			continue
		}
		text := bytes.Clone(unit.Text())
		for _, r := range s.strip {
			for j := r[0]; j < r[1]; j++ {
				if c := text[j]; c != '\n' && c != '\r' {
					text[j] = ' '
				}
			}
		}
		out[i] = text
	}
	return out
}

// newSurfacer readies a walk of a package's files.
func newSurfacer(files []*ast.File, units []*token.File) *surfacer {
	s := &surfacer{nonGeneric: map[string]bool{}}
	// A type the package declares without type parameters is the only
	// kind an extension or receiver of it is certainly not generic for.
	generic := map[string]bool{}
	for i, f := range files {
		unit := unitOf(f, units, i)
		for _, d := range topDecls(f) {
			name, params := nominal(d, unit)
			if name == "" {
				continue
			}
			if params {
				generic[name] = true
			} else {
				s.nonGeneric[name] = true
			}
		}
	}
	for name := range generic {
		delete(s.nonGeneric, name)
	}
	return s
}

// file finds the bodies of f to take out, and those a client compiles.
// It reports false for a file with no source.
func (s *surfacer) file(f *ast.File, unit *token.File) bool {
	if unit == nil {
		return false
	}
	s.unit, s.strip, s.kept = unit, s.strip[:0], s.kept[:0]
	for _, d := range topDecls(f) {
		s.top(d)
	}
	return true
}

type surfacer struct {
	nonGeneric map[string]bool
	unit       *token.File
	// strip is the bodies to take out of unit, as the offsets of the
	// braces around each, and kept the bodies left in.
	strip, kept [][2]int
}

// unitOf is the source of files[i].
func unitOf(f *ast.File, units []*token.File, i int) *token.File {
	if i < len(units) && units[i] != nil {
		return units[i]
	}
	return f.Unit
}

// topDecls is a file's declarations, in order.
func topDecls(f *ast.File) []ast.Decl {
	var out []ast.Decl
	for _, stmt := range f.Stmts {
		if d, ok := stmt.(*ast.DeclStmt); ok {
			out = append(out, d.D)
		}
	}
	return out
}

// nominal is a type declaration's name and whether it has type
// parameters, or "" for any other declaration.
func nominal(d ast.Decl, unit *token.File) (string, bool) {
	var id *ast.Ident
	var params *ast.GenericParams
	switch n := d.(type) {
	case *ast.StructDecl:
		id, params = n.Name, n.Generics
	case *ast.ClassDecl:
		id, params = n.Name, n.Generics
	case *ast.EnumDecl:
		id, params = n.Name, n.Generics
	case *ast.ActorDecl:
		id, params = n.Name, n.Generics
	}
	if id == nil || unit == nil {
		return "", false
	}
	return id.Name(unit), params != nil
}

// certainlyNonGeneric reports whether a written type names one of the
// package's own types that has no type parameters.
func (s *surfacer) certainlyNonGeneric(t ast.Type) bool {
	it, ok := t.(*ast.IdentType)
	return ok && it.Args == nil && it.Name != nil && s.nonGeneric[it.Name.Name(s.unit)]
}

func (s *surfacer) inlinable(attrs []*ast.Attr) bool {
	return ast.HasAttr(attrs, s.unit, ast.Inlinable)
}

func (s *surfacer) top(d ast.Decl) {
	switch n := d.(type) {
	case *ast.FuncDecl:
		if n.Generics != nil || s.inlinable(n.Attrs) ||
			(n.Recv != nil && !s.certainlyNonGeneric(n.Recv.Type)) {
			s.keep(n.Body)
			return
		}
		s.drop(n.Body)
	case *ast.StructDecl:
		s.typeBody(n.Generics, n.Body)
	case *ast.ClassDecl:
		s.typeBody(n.Generics, n.Body)
	case *ast.EnumDecl:
		s.typeBody(n.Generics, n.Body)
	case *ast.ActorDecl:
		s.typeBody(n.Generics, n.Body)
	case *ast.ExtensionDecl:
		if s.certainlyNonGeneric(n.Type) {
			s.members(n.Body)
		} else {
			s.keepMembers(n.Body)
		}
	case *ast.VarDecl:
		s.vars(n)
	}
}

// typeBody takes out what no client compiles of a type's members: all of
// them are compiled where the type is generic.
func (s *surfacer) typeBody(params *ast.GenericParams, body *ast.MemberBlock) {
	if params == nil {
		s.members(body)
	} else {
		s.keepMembers(body)
	}
}

// members takes out the bodies of a non-generic type's members that a
// client does not compile: all but methods and initializers with type
// parameters of their own, and @inlinable ones.
func (s *surfacer) members(body *ast.MemberBlock) {
	if body == nil {
		return
	}
	for _, m := range body.Members {
		switch n := m.(type) {
		case *ast.FuncDecl:
			if n.Generics == nil && !s.inlinable(n.Attrs) {
				s.drop(n.Body)
			} else {
				s.keep(n.Body)
			}
		case *ast.InitDecl:
			if n.Generics == nil && !s.inlinable(n.Attrs) {
				s.drop(n.Body)
			} else {
				s.keep(n.Body)
			}
		case *ast.DeinitDecl:
			s.drop(n.Body)
		case *ast.SubscriptDecl:
			if n.Generics == nil && !s.inlinable(n.Attrs) {
				s.drop(n.Body)
				s.accessors(n.Accessors, s.drop)
			} else {
				s.keep(n.Body)
				s.accessors(n.Accessors, s.keep)
			}
		case *ast.VarDecl:
			if !s.inlinable(n.Attrs) {
				s.vars(n)
			} else {
				s.keepVars(n)
			}
		case *ast.StructDecl:
			s.typeBody(n.Generics, n.Body)
		case *ast.ClassDecl:
			s.typeBody(n.Generics, n.Body)
		case *ast.EnumDecl:
			s.typeBody(n.Generics, n.Body)
		case *ast.ActorDecl:
			s.typeBody(n.Generics, n.Body)
		}
	}
}

// vars takes out the bodies of computed properties and property
// observers. A stored property's initializer stays: a client may infer
// the property's type from it.
func (s *surfacer) vars(d *ast.VarDecl) {
	for _, b := range d.Bindings {
		s.drop(b.Body)
		s.accessors(b.Accessors, s.drop)
	}
}

func (s *surfacer) keepVars(d *ast.VarDecl) {
	for _, b := range d.Bindings {
		s.keep(b.Body)
		s.accessors(b.Accessors, s.keep)
	}
}

func (s *surfacer) accessors(a *ast.AccessorBlock, mark func(*ast.CodeBlock)) {
	if a == nil {
		return
	}
	for _, acc := range a.Accessors {
		mark(acc.Body)
	}
}

// keepMembers records the bodies of a generic type's members, all of
// which a client compiles.
func (s *surfacer) keepMembers(body *ast.MemberBlock) {
	if body == nil {
		return
	}
	for _, m := range body.Members {
		switch n := m.(type) {
		case *ast.FuncDecl:
			s.keep(n.Body)
		case *ast.InitDecl:
			s.keep(n.Body)
		case *ast.DeinitDecl:
			s.keep(n.Body)
		case *ast.SubscriptDecl:
			s.keep(n.Body)
			s.accessors(n.Accessors, s.keep)
		case *ast.VarDecl:
			s.keepVars(n)
		case *ast.StructDecl:
			s.keepMembers(n.Body)
		case *ast.ClassDecl:
			s.keepMembers(n.Body)
		case *ast.EnumDecl:
			s.keepMembers(n.Body)
		case *ast.ActorDecl:
			s.keepMembers(n.Body)
		}
	}
}

// drop marks a code block's contents, between its braces, to be taken out.
func (s *surfacer) drop(b *ast.CodeBlock) {
	if r, ok := s.span(b); ok {
		s.strip = append(s.strip, r)
	}
}

// keep records a body a client compiles, which may read positions.
func (s *surfacer) keep(b *ast.CodeBlock) {
	if r, ok := s.span(b); ok {
		s.kept = append(s.kept, r)
	}
}

func (s *surfacer) span(b *ast.CodeBlock) ([2]int, bool) {
	if b == nil || !b.Lbrace.IsValid() || !b.Rbrace.IsValid() {
		return [2]int{}, false
	}
	lo, hi := s.unit.Offset(b.Lbrace)+1, s.unit.Offset(b.Rbrace)
	if lo < 0 || hi < lo || hi > s.unit.Size() {
		return [2]int{}, false
	}
	return [2]int{lo, hi}, true
}

// write hashes the unit's name and text, each marked body replaced by
// the lines it spans and, where kept text follows on its closing line,
// the column it closes at.
func (s *surfacer) write(h hash.Hash) {
	src := s.unit.Text()
	sort.Slice(s.strip, func(i, j int) bool { return s.strip[i][0] < s.strip[j][0] })
	fmt.Fprintf(h, "%d:%s\n", len(s.unit.Name()), s.unit.Name())
	// Past this offset nothing reads a position: no kept body, no #line.
	reads := -1
	for _, r := range s.kept {
		reads = max(reads, r[0])
	}
	// A #line in a body taken out reads nothing a client compiles, and
	// the pruned text has it blanked: only kept text counts.
	for _, w := range []string{"#line", "#column", "#sourceLocation"} {
		for at := len(src); ; {
			i := bytes.LastIndex(src[:at], []byte(w))
			if i < 0 || i <= reads {
				break
			}
			if !s.stripped(i) {
				reads = i
				break
			}
			at = i
		}
	}
	at := 0
	for _, r := range s.strip {
		lo, hi := r[0], r[1]
		if lo < at { // inside a body already taken out
			continue
		}
		h.Write(src[at:lo])
		if hi >= reads {
			h.Write([]byte{0, 0})
			at = hi
			continue
		}
		lines, col := 0, 0
		for _, c := range src[lo:hi] {
			col++
			if c == '\n' {
				lines++
				col = 0
			}
		}
		if !restOfLineBlank(src[hi+1:]) {
			fmt.Fprintf(h, "\x00%d:%d\x00", lines, col)
		} else {
			fmt.Fprintf(h, "\x00%d\x00", lines)
		}
		at = hi
	}
	h.Write(src[at:])
	h.Write([]byte{0})
}

// stripped reports whether offset i is inside a body taken out.
func (s *surfacer) stripped(i int) bool {
	for _, r := range s.strip {
		if r[0] <= i && i < r[1] {
			return true
		}
	}
	return false
}

// restOfLineBlank reports whether nothing but spaces and tabs precede
// the next newline.
func restOfLineBlank(b []byte) bool {
	for _, c := range b {
		switch c {
		case '\n', '\r':
			return true
		case ' ', '\t':
		default:
			return false
		}
	}
	return true
}
