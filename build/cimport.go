package build

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/vertex-language/ir"
	"github.com/vertex-language/vcx"
	"github.com/vertex-language/vcx/ast"
	"github.com/vertex-language/vcx/parser"
	"github.com/vertex-language/vcx/preprocessor"
	"github.com/vertex-language/vcx/sema"
	"github.com/vertex-language/vcx/types"

	"github.com/vertex-language/vsc/iface"
)

// CImport specifies a C-family module whose public headers are parsed
// to generate an imported Swift/Vertex module interface.
type CImport struct {
	Module string
	// Headers is the public header directory: every header under it is
	// part of the module, and only declarations written in one of them are
	// imported.
	Headers string
	// IncludeDirs are searched for what those headers include.
	IncludeDirs []string
	// Defines are the macros the target is compiled with, NAME or NAME=VALUE.
	Defines []string
	// CXX says the target is C++, whose functions are C symbols only where
	// they are declared extern "C".
	CXX    bool
	Target ir.Target
}

// CInterface writes a C module's interface, and names what it left out.
func CInterface(ci CImport) ([]byte, []string, error) {
	headersDir, err := filepath.Abs(ci.Headers)
	if err != nil {
		return nil, nil, err
	}
	headers, err := headerFiles(headersDir)
	if err != nil {
		return nil, nil, err
	}
	var tu bytes.Buffer
	for _, h := range headers {
		rel, _ := filepath.Rel(headersDir, h)
		fmt.Fprintf(&tu, "#include \"%s\"\n", filepath.ToSlash(rel))
	}

	name := targetName(ci.Target)
	c := &vcx.Compiler{
		Target:      name,
		IncludeDirs: append([]string{headersDir}, ci.IncludeDirs...),
		Defs:        ci.Defines,
	}
	file, _, err := c.Parse(vcx.Text(ci.Module+"-import.cpp", tu.Bytes()), parser.DefaultMode)
	if err != nil {
		return nil, nil, fmt.Errorf("reading the headers of %s: %w", ci.Module, err)
	}
	defer file.Release()
	tgt, err := vcx.TargetByName(name)
	if err != nil {
		return nil, nil, err
	}
	res, _ := sema.Analyze(file, types.ModelForTarget(tgt.Arch, tgt.OS))
	if res == nil {
		return nil, nil, fmt.Errorf("the headers of %s did not analyze", ci.Module)
	}
	site, _ := any(file.Unit).(interface {
		Site(ast.Tok) preprocessor.Site
	})

	m := cMapper{windows: strings.HasSuffix(ci.Target.Use(), "/windows")}
	var out bytes.Buffer
	fmt.Fprintf(&out, "// vertex-interface-format-version: %s\n", iface.FormatVersion)
	fmt.Fprintf(&out, "// vertex-module-name: %s\n", ci.Module)
	fmt.Fprintf(&out, "// Imported by vcx from the C headers of %s.\n\n", ci.Module)

	var skipped []string
	seen := map[string]bool{}
	for _, fn := range res.Declared {
		if fn == nil || fn.FuncType == nil || seen[fn.SymName] || fn.InClass != nil {
			continue
		}
		if site == nil || !declaredUnder(site.Site(fn.SymPos), headersDir) {
			continue
		}
		seen[fn.SymName] = true
		switch {
		case fn.Internal || fn.Body != nil:
			// Nothing to link against: a static or inline function's code is
			// in the header, and this compiler does not compile headers.
			skipped = append(skipped, fn.SymName+": defined in the header")
			continue
		case ci.CXX && !fn.ExternC:
			skipped = append(skipped, fn.SymName+": a C++ function, whose symbol is mangled")
			continue
		case fn.FuncType.Variadic:
			skipped = append(skipped, fn.SymName+": variadic")
			continue
		}
		decl, why := m.declaration(fn)
		if why != "" {
			skipped = append(skipped, fn.SymName+": "+why)
			continue
		}
		out.WriteString(decl)
	}
	return out.Bytes(), skipped, nil
}

// headerFiles is every header under dir, in a stable order.
func headerFiles(dir string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		switch filepath.Ext(path) {
		case ".h", ".hh", ".hpp", ".hxx":
			out = append(out, path)
		}
		return nil
	})
	sort.Strings(out)
	return out, err
}

// declaredUnder reports whether a declaration was written in a file under dir.
func declaredUnder(s preprocessor.Site, dir string) bool {
	if s.Origin == nil || s.Origin.File == nil {
		return false
	}
	path := s.Origin.File.Name()
	if !filepath.IsAbs(path) && s.Origin.Mount != nil {
		path = filepath.Join(s.Origin.Mount.Name, s.Origin.Path)
	}
	path = filepath.Clean(path)
	return strings.HasPrefix(path, dir+string(filepath.Separator))
}

type cMapper struct {
	windows bool
}

// declaration is a C function as a Swift declaration of its symbol, or why
// it is not one yet.
func (m cMapper) declaration(fn *sema.FuncSymbol) (string, string) {
	var params []string
	for i, p := range fn.FuncType.Params {
		t, ok := m.swiftType(p.Type)
		if !ok {
			return "", "a parameter is '" + p.Type.String() + "', which is not imported yet"
		}
		name := p.Name
		if !swiftIdentifier(name) {
			name = fmt.Sprintf("arg%d", i)
		}
		params = append(params, "_ "+name+": "+t)
	}
	result := ""
	if b, isBasic := fn.FuncType.Ret.(*types.Basic); !isBasic || b.K != types.Void {
		t, ok := m.swiftType(fn.FuncType.Ret)
		if !ok {
			return "", "it returns '" + fn.FuncType.Ret.String() + "', which is not imported yet"
		}
		result = " -> " + t
	}
	return fmt.Sprintf("@_silgen_name(\"%s\")\npublic func %s(%s)%s\n\n",
		fn.SymName, fn.SymName, strings.Join(params, ", "), result), ""
}

// swiftType is a C type in the type Swift imports it as.
//
// A qualifier on the value itself means nothing to a caller -- `const int n`
// is an Int32 -- and a pointer is what Swift's Clang importer makes of one:
// `const T *` an UnsafePointer<T>, `T *` an UnsafeMutablePointer<T>, and a
// pointer to void the raw pair. Implicitly unwrapped, because a header that
// says nothing about nullability says nothing: Swift will neither assume a
// pointer is never nil nor make every use of one check.
func (m cMapper) swiftType(t types.Type) (string, bool) {
	t = types.Unqualify(t)
	if p, ok := t.(*types.Pointer); ok {
		return m.pointerType(p)
	}
	b, ok := t.(*types.Basic)
	if !ok {
		return "", false
	}
	switch b.K {
	case types.Bool:
		return "Bool", true
	case types.Char:
		return "CChar", true
	case types.SChar:
		return "Int8", true
	case types.UChar, types.Char8:
		return "UInt8", true
	case types.Short:
		return "Int16", true
	case types.UShort, types.Char16:
		return "UInt16", true
	case types.Int:
		return "Int32", true
	case types.UInt:
		return "UInt32", true
	case types.Long:
		if m.windows {
			return "Int32", true
		}
		return "Int", true
	case types.ULong:
		if m.windows {
			return "UInt32", true
		}
		return "UInt", true
	case types.LongLong:
		return "Int64", true
	case types.ULongLong:
		return "UInt64", true
	case types.Float:
		return "Float", true
	case types.Double:
		return "Double", true
	}
	return "", false
}

// pointerType is a C pointer in the type Swift imports it as.
func (m cMapper) pointerType(p *types.Pointer) (string, bool) {
	constant := types.QualsOf(p.Elem)&types.QConst != 0
	elem := types.Unqualify(p.Elem)
	if b, ok := elem.(*types.Basic); ok && b.K == types.Void {
		if constant {
			return "UnsafeRawPointer!", true
		}
		return "UnsafeMutableRawPointer!", true
	}
	inner, ok := m.swiftType(elem)
	if !ok {
		return "", false
	}
	if constant {
		return "UnsafePointer<" + inner + ">!", true
	}
	return "UnsafeMutablePointer<" + inner + ">!", true
}

var identifierPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

var swiftKeywords = map[string]bool{
	"associatedtype": true, "class": true, "deinit": true, "enum": true, "extension": true,
	"fileprivate": true, "func": true, "import": true, "init": true, "inout": true, "internal": true,
	"let": true, "open": true, "operator": true, "private": true, "protocol": true, "public": true,
	"rethrows": true, "static": true, "struct": true, "subscript": true, "typealias": true, "var": true,
	"break": true, "case": true, "continue": true, "default": true, "defer": true, "do": true,
	"else": true, "fallthrough": true, "for": true, "guard": true, "if": true, "in": true,
	"repeat": true, "return": true, "switch": true, "where": true, "while": true, "as": true,
	"Any": true, "catch": true, "false": true, "is": true, "nil": true, "super": true, "self": true,
	"Self": true, "throw": true, "throws": true, "true": true, "try": true, "_": true,
}

func swiftIdentifier(name string) bool {
	return identifierPattern.MatchString(name) && !swiftKeywords[name]
}
