package build

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/vertex-language/ir"
	"github.com/vertex-language/vcx"
	"github.com/vertex-language/vcx/ast"
	"github.com/vertex-language/vcx/constexpr"
	"github.com/vertex-language/vcx/parser"
	"github.com/vertex-language/vcx/preprocessor"
	"github.com/vertex-language/vcx/sema"
	"github.com/vertex-language/vcx/types"
)

// A Native is the C++ half of a Vertex package folder: a C++ named
// module, whose interface unit's exports the package's Vertex sees.
//
// This is what the Clang importer is to Swift, over a module rather than
// headers. vcx reads the interface and says what it exports; the package
// gets Vertex declarations for them and a unit of `extern "C"` thunks
// that call them, so that everything the C++ ABI decides -- how a
// std::string_view is passed, what a symbol is called -- stays vcx's.
type Native struct {
	Dir string
	// Module is the C++ module's name: net.tcp.
	Module string
	// Interface is the module's interface unit.
	Interface string
	// Sources are every .cpp and .mm of the folder built for the target,
	// the interface unit among them.
	Sources []string
	Target  ir.Target
	// Modules are the other named modules the folder's units may import,
	// by name: their interface units.
	Modules map[string]string
	// Imports are the named modules the units import, other than this
	// one and its partitions: other packages' modules, and vertex.task.
	Imports []string
	// Libraries and Frameworks are what the units' link pragmas name
	// (`#pragma vertex framework("AppKit")`), which a program using the
	// package links.
	Libraries  []string
	Frameworks []string
}

// FindNative reads what module the C++ sources of a folder are. Exactly
// one of them is the module's interface unit (`export module M;`), and
// every other is one of M's units or a unit of no module at all.
func FindNative(dir string, sources []string, target ir.Target) (*Native, error) {
	n := &Native{Dir: dir, Sources: sources, Target: target}
	c := n.compiler()
	for _, src := range sources {
		sc, diags, err := c.Scan(vcx.File(src))
		if err != nil {
			return nil, err
		}
		if vcx.HasErrors(diags) {
			return nil, &vcx.DiagnosticError{Diagnostics: diags}
		}
		for _, l := range sc.Links {
			if l.Kind == preprocessor.LinkFramework {
				n.Frameworks = appendNew(n.Frameworks, l.Name)
			} else {
				n.Libraries = appendNew(n.Libraries, l.Name)
			}
		}
		for _, d := range sc.Modules {
			switch d.Kind {
			case preprocessor.Interface:
				name, part, _ := strings.Cut(d.Name, ":")
				if part != "" {
					continue
				}
				if n.Interface != "" {
					return nil, fmt.Errorf("%s: both %s and %s are the interface of a module; a package has one",
						dir, filepath.Base(n.Interface), filepath.Base(src))
				}
				n.Module, n.Interface = name, src
			case preprocessor.Implementation:
				name, _, _ := strings.Cut(d.Name, ":")
				if n.Module != "" && name != n.Module {
					return nil, fmt.Errorf("%s: %s is a unit of module %s, not of %s", dir, filepath.Base(src), name, n.Module)
				}
			case preprocessor.Import:
				if d.Name != "" && !strings.HasPrefix(d.Name, ":") {
					n.Imports = appendNew(n.Imports, d.Name)
				}
			}
		}
	}
	if n.Interface == "" {
		return nil, fmt.Errorf("%s: the C++ in this package declares no module: one file must begin its declarations with `export module <name>;`", dir)
	}
	// Its own module, which an implementation unit names, is not an import.
	var imports []string
	for _, name := range n.Imports {
		if name != n.Module {
			imports = append(imports, name)
		}
	}
	n.Imports = imports
	return n, nil
}

// TaskModule is the name of the module that is the runtime's task ABI to a
// package's C++: `import vertex.task;` declares what of stdlib/ABI.md is a
// plain C function. The rest -- vertex_task_wait_fd, sleep, yield -- are
// async entries, which only Vertex code can call: a bridge returns "would
// block" and the package's Vertex waits.
const TaskModule = "vertex.task"

// taskModuleSource is that module's interface unit. The functions are the
// runtime's, which every program links.
const taskModuleSource = `// The Vertex runtime's task ABI, for packages' C++ (stdlib/ABI.md): its
// plain C functions. Waiting (vertex_task_wait_fd, sleep, yield) is an
// async call that only Vertex makes.
module;
#include <cstdint>
export module vertex.task;

export extern "C" {
    // How many workers the pool has, starting it if it has not started;
    // 0 where there is none. For a package that spreads work over the pool.
    int32_t vertex_task_workers() noexcept;
}
`

// TaskModuleFile writes the vertex.task interface unit under work, once,
// and is its path.
func TaskModuleFile(work string) (string, error) {
	path := filepath.Join(work, "native", "vertex.task.cppm")
	if data, err := os.ReadFile(path); err == nil && string(data) == taskModuleSource {
		return path, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	return path, os.WriteFile(path, []byte(taskModuleSource), 0o644)
}

// ModuleNameFor is the C++ module name a package folder imported by path
// must declare: the path with its slashes as dots, less the host and owner
// of a path that names a repository -- net/tcp is net.tcp, and
// github.com/you/thing/x is thing.x.
func ModuleNameFor(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) >= 3 && strings.Contains(parts[0], ".") {
		parts = parts[2:]
	}
	for i, p := range parts {
		parts[i] = strings.NewReplacer("-", "_", ".", "_").Replace(p)
	}
	return strings.Join(parts, ".")
}

func (n *Native) compiler() *vcx.Compiler {
	modules := map[string]string{}
	for name, file := range n.Modules {
		modules[name] = file
	}
	if n.Module != "" {
		modules[n.Module] = n.Interface
	}
	return &vcx.Compiler{
		Target:      targetName(n.Target),
		Std:         vcx.Cxx23,
		IncludeDirs: []string{n.Dir},
		Modules:     modules,
	}
}

// Bindings are what the package's Vertex sees of the module -- a Vertex
// source of declarations, one for each export it can import -- and the
// C++ unit of thunks those declarations call. public says the exports are
// the package's API (a folder of C++ alone); otherwise they are internal
// to the package, whose .vs files are its API. skipped names each export
// left out and why.
func (n *Native) Bindings(pkgName string, public bool) (vs, thunks []byte, skipped []string, err error) {
	c := n.compiler()
	file, diags, err := c.Parse(vcx.File(n.Interface), parser.DefaultMode)
	if err != nil {
		return nil, nil, nil, err
	}
	if vcx.HasErrors(diags) {
		return nil, nil, nil, &vcx.DiagnosticError{Diagnostics: diags}
	}
	defer file.Release()
	tgt, err := vcx.TargetByName(targetName(n.Target))
	if err != nil {
		return nil, nil, nil, err
	}
	res, sdiags := sema.Analyze(file, types.ModelForTarget(tgt.Arch, tgt.OS))
	var semaDiags []vcx.Diagnostic
	for _, d := range sdiags {
		semaDiags = append(semaDiags, vcx.Diagnostic{Severity: d.Severity, Site: d.Site, Message: d.Message})
	}
	if vcx.HasErrors(semaDiags) {
		return nil, nil, nil, &vcx.DiagnosticError{Diagnostics: semaDiags}
	}

	g := &bindingGen{
		res:     res,
		n:       n,
		unit:    file.Unit,
		public:  public,
		windows: strings.HasSuffix(n.Target.Use(), "/windows"),
		enums:   map[*types.Enum]string{},
	}
	g.exports = exportSpans(file.Decls, nil, file.Unit)
	g.collect(res.GlobalScope, nil, map[*sema.Scope]bool{})
	for _, fn := range res.Declared {
		if fn.InClass != nil || fn.Template != nil || fn.TemplateOf != nil || !g.exported(fn.SymPos) {
			continue
		}
		g.addFunc(fn)
	}
	vs, thunks = g.emit(pkgName)
	return vs, thunks, g.skipped, nil
}

type span struct{ lo, hi ast.Tok }

// exportSpans are the token ranges of every export-declaration, at any
// depth of namespaces: the module's own, and not those of a module it
// imports, whose interface is read into the same unit.
func exportSpans(decls []ast.Decl, out []span, unit ast.Unit) []span {
	site, _ := unit.(interface {
		Site(ast.Tok) preprocessor.Site
	})
	for _, d := range decls {
		switch d := d.(type) {
		case *ast.ExportDecl:
			if site != nil {
				if org := site.Site(d.Pos()).Origin; org != nil && org.Module != "" {
					continue
				}
			}
			out = append(out, span{d.Pos(), d.End()})
		case *ast.NamespaceDecl:
			out = exportSpans(d.Decls, out, unit)
		}
	}
	return out
}

type bindingGen struct {
	res     *sema.Result
	n       *Native
	unit    ast.Unit
	public  bool
	windows bool
	exports []span

	enums   map[*types.Enum]string // exported enum → its Vertex name
	enumOut []enumOut
	consts  []constOut
	funcs   []funcOut
	skipped []string
	seenFn  map[string]bool
}

type enumOut struct {
	scope  []string // Vertex namespaces
	name   string
	e      *types.Enum
	raw    string // the Vertex raw type
	scoped bool
}

type constOut struct {
	scope []string
	name  string
	typ   string
	val   int64
}

type funcOut struct {
	scope  []string
	name   string // Vertex name
	thunk  string // the extern "C" symbol
	cxx    string // the qualified C++ callee
	params []paramOut
	ret    typeMap
}

type paramOut struct {
	name string
	t    typeMap
}

// A typeMap is one C++ type as the two sides of a thunk see it.
type typeMap struct {
	vertex string   // the type the Vertex declaration has
	raw    []string // the types the thunk's Vertex declaration takes it as
	cxx    []string // the thunk's C++ parameter types, one per raw
	// toRaw is the Vertex arguments for a value named v; fromRaw the
	// Vertex value of a result r. Empty is v, r as they are.
	toRaw   func(v string) []string
	fromRaw func(r string) string
	// toCxx is the C++ argument for thunk parameters a0.. (by index);
	// fromCxx the thunk's return of a C++ result expression.
	toCxx   func(args []string) string
	fromCxx func(e string) string
	void    bool
}

func (g *bindingGen) exported(pos ast.Tok) bool {
	for _, s := range g.exports {
		if pos >= s.lo && pos < s.hi {
			return true
		}
	}
	return false
}

// vertexScope is the Vertex namespace path of a C++ one: a namespace that
// is the module itself -- math for math, or net::tcp or tcp for net.tcp --
// is the package, and the ones inside it are nested.
func (g *bindingGen) vertexScope(path []string) []string {
	mod := strings.Split(g.n.Module, ".")
	for _, prefix := range [][]string{mod, mod[len(mod)-1:]} {
		if len(path) >= len(prefix) && equalStrings(path[:len(prefix)], prefix) {
			return path[len(prefix):]
		}
	}
	return path
}

// collect finds the exported enums and constants of a scope and the
// namespaces in it.
func (g *bindingGen) collect(scope *sema.Scope, path []string, seen map[*sema.Scope]bool) {
	if scope == nil || seen[scope] {
		return
	}
	seen[scope] = true
	names := make([]string, 0, len(scope.Symbols))
	for name := range scope.Symbols {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		for _, sym := range scope.Symbols[name] {
			switch s := sym.(type) {
			case *sema.NamespaceSymbol:
				g.collect(s.InnerScope, append(append([]string(nil), path...), s.SymName), seen)
			case *sema.EnumSymbol:
				if !g.exported(s.SymPos) || s.Enum == nil || !s.Enum.Complete || s.SymName == "" {
					continue
				}
				raw, ok := g.basic(s.Enum.Underlying)
				if !ok {
					g.skip(s.SymName, "its underlying type is not an integer Vertex has")
					continue
				}
				g.enums[s.Enum] = s.SymName
				g.enumOut = append(g.enumOut, enumOut{scope: g.vertexScope(path), name: s.SymName, e: s.Enum, raw: raw, scoped: s.Enum.Scoped})
			case *sema.VarSymbol:
				if !g.exported(s.SymPos) || !s.Constexpr {
					continue
				}
				val, known := s.KnownValue, s.HasKnownValue
				if !known && s.Init != nil && g.res.Info != nil {
					val, known = g.res.Info.Consts[s.Init]
				}
				if !known && s.Init != nil {
					if v, err := constexpr.NewContext(g.unit, g.res.Model).EvalInt(s.Init); err == nil {
						val, known = v, true
					}
				}
				t, ok := g.basic(s.SymType)
				if !ok || !known {
					g.skip(s.SymName, "a constant of a type that is not imported yet")
					continue
				}
				g.consts = append(g.consts, constOut{scope: g.vertexScope(path), name: s.SymName, typ: t, val: val})
			}
		}
	}
}

func (g *bindingGen) skip(name, why string) {
	g.skipped = append(g.skipped, name+": "+why)
}

func (g *bindingGen) addFunc(fn *sema.FuncSymbol) {
	path := fn.SymScope.Path()
	key := strings.Join(path, "::") + "::" + fn.SymName + fn.FuncType.String()
	if g.seenFn == nil {
		g.seenFn = map[string]bool{}
	}
	if g.seenFn[key] {
		return
	}
	g.seenFn[key] = true
	if !swiftIdentifier(fn.SymName) || fn.FuncType.Variadic {
		g.skip(fn.SymName, "not a function Vertex can name")
		return
	}
	f := funcOut{
		scope: g.vertexScope(path),
		name:  fn.SymName,
		cxx:   "::" + strings.Join(append(append([]string(nil), path...), fn.SymName), "::"),
	}
	for i, p := range fn.FuncType.Params {
		m, why := g.param(p.Type)
		if why != "" {
			g.skip(fn.SymName, fmt.Sprintf("parameter %d is %s: %s", i+1, p.Type.String(), why))
			return
		}
		name := p.Name
		if name == "" && i < len(fn.Params) && fn.Params[i] != nil {
			name = fn.Params[i].SymName
		}
		if !swiftIdentifier(name) {
			name = fmt.Sprintf("arg%d", i)
		}
		f.params = append(f.params, paramOut{name: name, t: m})
	}
	ret, why := g.result(fn.FuncType.Ret)
	if why != "" {
		g.skip(fn.SymName, fmt.Sprintf("it returns %s: %s", fn.FuncType.Ret.String(), why))
		return
	}
	f.ret = ret
	f.thunk = fmt.Sprintf("__vs_%s_%s_%d", strings.ReplaceAll(g.n.Module, ".", "_"), fn.SymName, len(g.funcs))
	g.funcs = append(g.funcs, f)
}

// basic is the Vertex type of a C++ arithmetic type, by width.
func (g *bindingGen) basic(t types.Type) (string, bool) {
	b, ok := types.Unqualify(t).(*types.Basic)
	if !ok {
		return "", false
	}
	switch b.K {
	case types.Bool:
		return "bool", true
	case types.Char:
		return "CChar", true
	case types.SChar:
		return "int8", true
	case types.UChar, types.Char8:
		return "uint8", true
	case types.Short:
		return "int16", true
	case types.UShort, types.Char16:
		return "uint16", true
	case types.Int:
		return "int32", true
	case types.UInt, types.Char32:
		return "uint32", true
	case types.Long:
		if g.windows {
			return "int32", true
		}
		return "int", true
	case types.ULong:
		if g.windows {
			return "uint32", true
		}
		return "uint", true
	case types.LongLong:
		return "int64", true
	case types.ULongLong:
		return "uint64", true
	case types.Float:
		return "float32", true
	case types.Double:
		return "float64", true
	}
	return "", false
}

// cxxBasic is the C++ spelling of a basic type for the thunk unit.
func cxxBasic(t types.Type) string {
	b := types.Unqualify(t).(*types.Basic)
	switch b.K {
	case types.Bool:
		return "bool"
	case types.Char:
		return "char"
	case types.SChar:
		return "signed char"
	case types.UChar:
		return "unsigned char"
	case types.Char8:
		return "char8_t"
	case types.Short:
		return "short"
	case types.UShort:
		return "unsigned short"
	case types.Char16:
		return "char16_t"
	case types.Int:
		return "int"
	case types.UInt:
		return "unsigned int"
	case types.Char32:
		return "char32_t"
	case types.Long:
		return "long"
	case types.ULong:
		return "unsigned long"
	case types.LongLong:
		return "long long"
	case types.ULongLong:
		return "unsigned long long"
	case types.Float:
		return "float"
	case types.Double:
		return "double"
	}
	return "void"
}

func plain(vertex, cxx string) typeMap {
	return typeMap{vertex: vertex, raw: []string{vertex}, cxx: []string{cxx}}
}

// param is how a parameter of C++ type t crosses.
func (g *bindingGen) param(t types.Type) (typeMap, string) {
	if m, ok := g.scalar(t); ok {
		return m, ""
	}
	if r, ok := recordOf(t); ok {
		elem, isStd := stdTemplate(r)
		switch {
		case isStd == "basic_string_view":
			if b, ok := types.Unqualify(elem).(*types.Basic); !ok || b.K != types.Char {
				return typeMap{}, "only a string_view of char is imported"
			}
			return typeMap{
				vertex: "string",
				raw:    []string{"UnsafePointer<CChar>?", "int"},
				cxx:    []string{"const char*", "long long"},
				toRaw:  func(v string) []string { return []string{v, v + ".utf8.count"} },
				toCxx: func(a []string) string {
					return fmt.Sprintf("std::string_view(%s, static_cast<std::size_t>(%s))", a[0], a[1])
				},
			}, ""
		case isStd == "span":
			if len(r.TemplateArgs) > 1 && !r.TemplateArgs[1].IsType && uint64(r.TemplateArgs[1].Val) != ^uint64(0) {
				return typeMap{}, "a span of fixed extent is not imported"
			}
			constant := types.QualsOf(elem)&types.QConst != 0
			inner, ok := g.scalar(types.Unqualify(elem))
			if !ok || len(inner.raw) != 1 {
				return typeMap{}, "a span of this element type is not imported"
			}
			buf, ptr, cptr := "UnsafeMutableBufferPointer", "UnsafeMutablePointer", inner.cxx[0]+"*"
			if constant {
				buf, ptr, cptr = "UnsafeBufferPointer", "UnsafePointer", "const "+inner.cxx[0]+"*"
			}
			return typeMap{
				vertex: buf + "<" + inner.vertex + ">",
				raw:    []string{ptr + "<" + inner.vertex + ">?", "int"},
				cxx:    []string{cptr, "long long"},
				toRaw:  func(v string) []string { return []string{v + ".baseAddress", v + ".count"} },
				toCxx: func(a []string) string {
					return fmt.Sprintf("std::span<%s>(%s, static_cast<std::size_t>(%s))", strings.TrimSuffix(cptr, "*"), a[0], a[1])
				},
			}, ""
		}
		return typeMap{}, "classes are not imported yet"
	}
	return typeMap{}, "not imported yet"
}

// result is how a result of C++ type t crosses.
func (g *bindingGen) result(t types.Type) (typeMap, string) {
	if b, ok := types.Unqualify(t).(*types.Basic); ok && b.K == types.Void {
		return typeMap{void: true}, ""
	}
	if m, ok := g.scalar(t); ok {
		return m, ""
	}
	return typeMap{}, "not imported as a result yet"
}

// scalar is an arithmetic type, an exported enum, or a pointer to one of
// those (or to void, or to such a pointer).
func (g *bindingGen) scalar(t types.Type) (typeMap, bool) {
	u := types.Unqualify(t)
	if v, ok := g.basic(u); ok {
		return plain(v, cxxBasic(u)), true
	}
	if e, ok := u.(*types.Enum); ok {
		name, exported := g.enums[e]
		raw, ok := g.basic(e.Underlying)
		if !ok {
			return typeMap{}, false
		}
		cxxRaw := cxxBasic(e.Underlying)
		qualified := "::" + strings.Join(append(append([]string(nil), e.Scopes...), e.Name), "::")
		m := typeMap{
			vertex:  raw,
			raw:     []string{raw},
			cxx:     []string{cxxRaw},
			toCxx:   func(a []string) string { return "static_cast<" + qualified + ">(" + a[0] + ")" },
			fromCxx: func(x string) string { return "static_cast<" + cxxRaw + ">(" + x + ")" },
		}
		if exported && e.Scoped {
			// An enum class is a Vertex enum over its raw values.
			m.vertex = g.qualifiedEnum(e, name)
			m.toRaw = func(v string) []string { return []string{v + ".rawValue"} }
			m.fromRaw = func(r string) string { return m.vertex + "(rawValue: " + r + ")!" }
		}
		return m, true
	}
	if p, ok := u.(*types.Pointer); ok {
		constant := types.QualsOf(p.Elem)&types.QConst != 0
		elem := types.Unqualify(p.Elem)
		if b, ok := elem.(*types.Basic); ok && b.K == types.Void {
			if constant {
				return plain("UnsafeRawPointer?", "const void*"), true
			}
			return plain("UnsafeMutableRawPointer?", "void*"), true
		}
		// A pointer to an arithmetic type or to another such pointer: the
		// same type on both sides, so the thunk passes it through.
		if _, isEnum := elem.(*types.Enum); isEnum {
			return typeMap{}, false
		}
		inner, ok := g.scalar(elem)
		if !ok || len(inner.raw) != 1 || inner.toCxx != nil {
			return typeMap{}, false
		}
		if constant {
			return plain("UnsafePointer<"+inner.raw[0]+">?", "const "+inner.cxx[0]+"*"), true
		}
		return plain("UnsafeMutablePointer<"+inner.raw[0]+">?", inner.cxx[0]+"*"), true
	}
	return typeMap{}, false
}

func (g *bindingGen) qualifiedEnum(e *types.Enum, name string) string {
	for _, out := range g.enumOut {
		if out.e == e {
			return strings.Join(append(append([]string(nil), out.scope...), name), ".")
		}
	}
	return name
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// recordOf is the class a type names, through the specialization a
// template-id like std::span<const int> is written as.
func recordOf(t types.Type) (*types.Record, bool) {
	switch u := types.Unqualify(t).(type) {
	case *types.Record:
		return u, true
	case *types.TemplateSpecialization:
		if r, ok := types.Unqualify(u.Type).(*types.Record); ok {
			if len(r.TemplateArgs) == 0 {
				copied := *r
				copied.TemplateArgs = u.Args
				return &copied, true
			}
			return r, true
		}
	}
	return nil, false
}

// stdTemplate is a specialization of a class template of namespace std --
// its name, and its first type argument -- or "" for any other class.
func stdTemplate(r *types.Record) (types.Type, string) {
	if len(r.Scopes) == 0 || r.Scopes[0] != "std" || len(r.TemplateArgs) == 0 || !r.TemplateArgs[0].IsType {
		return nil, ""
	}
	return r.TemplateArgs[0].Type, r.Name
}

// emit writes the two files.
func (g *bindingGen) emit(pkgName string) ([]byte, []byte) {
	access := ""
	if g.public {
		access = "public "
	}

	// Vertex: one tree of namespaces, as caseless enums, holding the enums,
	// constants and functions; the thunks' declarations at the top level.
	type node struct {
		kids  map[string]*node
		order []string
		body  bytes.Buffer
	}
	root := &node{kids: map[string]*node{}}
	at := func(scope []string) *node {
		n := root
		for _, s := range scope {
			k, ok := n.kids[s]
			if !ok {
				k = &node{kids: map[string]*node{}}
				n.kids[s] = k
				n.order = append(n.order, s)
			}
			n = k
		}
		return n
	}
	for _, e := range g.enumOut {
		b := &at(e.scope).body
		if e.scoped {
			fmt.Fprintf(b, "%senum %s: %s {\n", access, e.name, e.raw)
			seen := map[int64]bool{}
			for _, en := range e.e.Enumerators {
				if seen[en.Val] {
					continue
				}
				seen[en.Val] = true
				fmt.Fprintf(b, "    case %s = %d\n", vertexName(en.Name), en.Val)
			}
			b.WriteString("}\n\n")
			continue
		}
		// An unscoped enum is a set of integer constants, which C++ code
		// combines and compares as integers: so is it here.
		fmt.Fprintf(b, "%senum %s {\n", access, e.name)
		for _, en := range e.e.Enumerators {
			fmt.Fprintf(b, "    %sstatic let %s: %s = %d\n", access, vertexName(en.Name), e.raw, en.Val)
		}
		b.WriteString("}\n\n")
	}
	for _, c := range g.consts {
		b := &at(c.scope).body
		if len(c.scope) > 0 {
			fmt.Fprintf(b, "%sstatic let %s: %s = %d\n\n", access, vertexName(c.name), c.typ, c.val)
		} else {
			fmt.Fprintf(b, "%slet %s: %s = %d\n\n", access, vertexName(c.name), c.typ, c.val)
		}
	}

	var decls bytes.Buffer
	var cxx bytes.Buffer
	fmt.Fprintf(&cxx, "// Generated by vsc: the C ABI the Vertex side of package %s calls module %s through.\n", pkgName, g.n.Module)
	cxx.WriteString("module;\n#include <cstddef>\n#include <span>\n#include <string_view>\n")
	fmt.Fprintf(&cxx, "module %s;\n\n", g.n.Module)

	for _, f := range g.funcs {
		// The thunk's Vertex declaration.
		var rawParams, cxxParams, vsParams, rawArgs, cxxArgs []string
		for i, p := range f.params {
			vsParams = append(vsParams, "_ "+vertexName(p.name)+": "+p.t.vertex)
			var names []string
			for j, r := range p.t.raw {
				rawParams = append(rawParams, fmt.Sprintf("_ a%d_%d: %s", i, j, r))
				n := fmt.Sprintf("a%d_%d", i, j)
				cxxParams = append(cxxParams, p.t.cxx[j]+" "+n)
				names = append(names, n)
			}
			if p.t.toRaw != nil {
				rawArgs = append(rawArgs, p.t.toRaw(vertexName(p.name))...)
			} else {
				rawArgs = append(rawArgs, vertexName(p.name))
			}
			if p.t.toCxx != nil {
				cxxArgs = append(cxxArgs, p.t.toCxx(names))
			} else {
				cxxArgs = append(cxxArgs, names[0])
			}
		}
		result, cxxResult := "", "void"
		if !f.ret.void {
			result = " -> " + f.ret.raw[0]
			cxxResult = f.ret.cxx[0]
		}
		fmt.Fprintf(&decls, "@_silgen_name(\"%s\")\nfunc %s(%s)%s\n\n", f.thunk, f.thunk, strings.Join(rawParams, ", "), result)

		call := f.cxx + "(" + strings.Join(cxxArgs, ", ") + ")"
		if f.ret.void {
			fmt.Fprintf(&cxx, "extern \"C\" void %s(%s) noexcept { %s; }\n", f.thunk, strings.Join(cxxParams, ", "), call)
		} else {
			ret := call
			if f.ret.fromCxx != nil {
				ret = f.ret.fromCxx(call)
			}
			fmt.Fprintf(&cxx, "extern \"C\" %s %s(%s) noexcept { return %s; }\n", cxxResult, f.thunk, strings.Join(cxxParams, ", "), ret)
		}

		// The Vertex function over it.
		b := &at(f.scope).body
		static := ""
		if len(f.scope) > 0 {
			static = "static "
		}
		vsResult := ""
		if !f.ret.void {
			vsResult = " -> " + f.ret.vertex
		}
		body := f.thunk + "(" + strings.Join(rawArgs, ", ") + ")"
		if f.ret.fromRaw != nil {
			body = f.ret.fromRaw(body)
		}
		fmt.Fprintf(b, "%s%sfunc %s(%s)%s {\n    return %s\n}\n\n", access, static, vertexName(f.name), strings.Join(vsParams, ", "), vsResult, body)
	}

	var vs bytes.Buffer
	fmt.Fprintf(&vs, "// Generated by vsc from the exports of C++ module %s (%s).\n", g.n.Module, filepath.Base(g.n.Interface))
	fmt.Fprintf(&vs, "package %s\n\n", pkgName)
	var write func(n *node, name string, depth int)
	write = func(n *node, name string, depth int) {
		indent := strings.Repeat("    ", depth)
		if name != "" {
			fmt.Fprintf(&vs, "%s%senum %s {\n", strings.Repeat("    ", depth-1), access, vertexName(name))
		}
		for _, line := range strings.Split(strings.TrimRight(n.body.String(), "\n"), "\n") {
			if line == "" {
				vs.WriteString("\n")
				continue
			}
			vs.WriteString(indent + line + "\n")
		}
		for _, k := range n.order {
			write(n.kids[k], k, depth+1)
		}
		if name != "" {
			fmt.Fprintf(&vs, "%s}\n\n", strings.Repeat("    ", depth-1))
		}
	}
	write(root, "", 0)
	vs.WriteString("\n")
	vs.Write(decls.Bytes())
	return vs.Bytes(), cxx.Bytes()
}

// vertexName is a C++ name as a Vertex identifier, backquoted where it is
// a keyword.
func vertexName(name string) string {
	if swiftKeywords[name] {
		return "`" + name + "`"
	}
	return name
}

// Objects compiles the folder's C++ and the thunk unit, and says what the
// link needs for them. Objects are kept under work, keyed by everything
// that went into them, as a C-family target's are.
func (n *Native) Objects(thunks []byte, work string) ([]Input, Linkage, error) {
	need := Linkage{CXX: true, Libraries: n.Libraries, Frameworks: n.Frameworks}
	for _, src := range n.Sources {
		need.ObjC = need.ObjC || strings.EqualFold(filepath.Ext(src), ".mm")
	}
	c := n.compiler()
	key := n.cacheKey(thunks)
	thunkPath := filepath.Join(work, "native", key, "__vs_thunks.cpp")
	if err := os.MkdirAll(filepath.Dir(thunkPath), 0o755); err != nil {
		return nil, need, err
	}
	if err := os.WriteFile(thunkPath, thunks, 0o644); err != nil {
		return nil, need, err
	}
	var objs []Input
	prefix := strings.ReplaceAll(n.Module, ".", "_")
	for _, src := range append(append([]string(nil), n.Sources...), thunkPath) {
		name := prefix + "_" + strings.TrimSuffix(filepath.Base(src), filepath.Ext(src)) + ".o"
		cached := filepath.Join(work, "native", key, name)
		if data, err := os.ReadFile(cached); err == nil {
			objs = append(objs, Input{Name: name, Data: data})
			continue
		}
		data, diags, err := c.Object(vcx.File(src))
		if err == nil && vcx.HasErrors(diags) {
			err = &vcx.DiagnosticError{Diagnostics: diags}
		}
		if err != nil {
			return nil, need, fmt.Errorf("%s: %w", src, err)
		}
		_ = os.WriteFile(cached, data, 0o644)
		objs = append(objs, Input{Name: name, Data: data})
	}
	return objs, need, nil
}

// cacheKey names what the objects are made from: the target, the folder's
// files and the modules it may import.
func (n *Native) cacheKey(thunks []byte) string {
	h := sha256.New()
	fmt.Fprintf(h, "vsc-native-1\n%s\n%s\n", targetName(n.Target), compilerIdentity())
	h.Write(thunks)
	var files []string
	entries, _ := os.ReadDir(n.Dir)
	for _, e := range entries {
		if !e.IsDir() {
			files = append(files, filepath.Join(n.Dir, e.Name()))
		}
	}
	for _, f := range n.Modules {
		files = append(files, f)
	}
	sort.Strings(files)
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		fmt.Fprintf(h, "F %s %s\n", f, strconv.Itoa(len(data)))
		h.Write(data)
	}
	return hex.EncodeToString(h.Sum(nil))[:24]
}

// compilerIdentity names the compiler doing the build -- the executable,
// by path, size and time -- so that objects a different vcx made are not
// taken for this one's.
func compilerIdentity() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	info, err := os.Stat(exe)
	if err != nil {
		return exe
	}
	return fmt.Sprintf("%s %d %d", exe, info.Size(), info.ModTime().UnixNano())
}
