package gen

import (
	"github.com/vertex-language/vsc/analyzer"
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/core"
	"github.com/vertex-language/vsc/internal/sil"
	"github.com/vertex-language/vsc/token"
	"github.com/vertex-language/vsc/types"
)

// Interoperability attributes @_silgen_name and @_cdecl customize exported and imported symbol names for C interoperability.

const (
	attrSilgenName = "_silgen_name"
	// attrBuiltin is the core's: a function declared @_builtin("int_sqrt")
	// is that one machine instruction, named for its operand's type as
	// SIL names it -- int_sqrt_FPIEEE64.
	attrBuiltin = "_builtin"
	attrCDecl   = "_cdecl"
	attrObjC    = "objc"
)

// asmName returns the symbol named by an attribute and whether it was present.
func (g *gen) asmName(attrs []*ast.Attr, want string) (string, bool) {
	return g.asmNameIn(g.file, attrs, want)
}

// asmNameIn is asmName for attributes written in another file than the
// one being lowered: the built-in module's, whose declarations a call
// in this file names.
func (g *gen) asmNameIn(file *token.File, attrs []*ast.Attr, want string) (string, bool) {
	for _, a := range attrs {
		if a == nil || a.Name == nil {
			continue
		}
		id, ok := a.Name.(*ast.IdentType)
		if !ok || id.Name == nil || string(file.Slice(id.Name.Pos(), id.Name.End())) != want {
			continue
		}
		var segments []string
		for _, t := range a.Tokens {
			if t.Kind == token.STRING_SEGMENT {
				segments = append(segments, string(file.Slice(t.Pos, t.End)))
			}
		}
		if len(segments) == 1 && segments[0] != "" {
			return segments[0], true
		}
		return "", true
	}
	return "", false
}

// attrNameOf is an attribute's spelling.
func attrNameOf(g *gen, a *ast.Attr) string {
	id, ok := a.Name.(*ast.IdentType)
	if !ok || id.Name == nil {
		return ""
	}
	return g.text(id.Name)
}

// funcAttrs returns the attributes on a function symbol's declaration.
func funcAttrs(sym *analyzer.FuncSymbol) []*ast.Attr {
	if sym == nil {
		return nil
	}
	d, ok := sym.Decl().(*ast.FuncDecl)
	if !ok {
		return nil
	}
	return d.Attrs
}

// silgenName is the symbol `@_silgen_name` gives a function, if it
// has one. The empty string with true is the attribute written
// without a name, which the definition reports.
func (g *gen) silgenName(sym *analyzer.FuncSymbol) (string, bool) {
	file := g.file
	// Declared in another file of this module, whose text the
	// attribute's positions are measured against.
	if d, ok := sym.Decl().(*ast.FuncDecl); ok {
		if f := g.fileOf(d); f != nil {
			file = f
		}
	}
	if u := g.info.ImportedUnits[sym]; u != nil {
		// Declared in an imported interface, whose text the
		// attribute's positions are measured against.
		file = u
	}
	if g.info.Imported[sym] == "Swift" {
		// Declared by the built-in module, so its attributes are
		// spelled in core's text rather than this file's.
		_, file, _ = core.Files()
	}
	return g.asmNameIn(file, funcAttrs(sym), attrSilgenName)
}

// interopAttrs validates interop attributes on a function declaration.
func (g *gen) interopAttrs(d *ast.FuncDecl) bool {
	if name, has := g.asmName(d.Attrs, attrSilgenName); has && name == "" {
		g.errorAt(d, "'@_silgen_name' needs the symbol to use, as in "+
			"'@_silgen_name(\"abs\")'")
		return false
	}
	if name, has := g.asmName(d.Attrs, attrCDecl); has && name == "" {
		g.errorAt(d, "'@_cdecl' needs the symbol to export, as in "+
			"'@_cdecl(\"vs_add\")'")
		return false
	}
	if _, has := g.asmName(d.Attrs, attrObjC); has {
		g.errorAt(d, "cannot lower '@objc' yet: it asks for an Objective-C "+
			"entry point, which needs the Objective-C runtime and the "+
			"metadata that goes with it")
		return false
	}
	return true
}

// cdeclThunk emits a C-callable thunk with C calling convention that forwards to the implementation.
func (g *gen) cdeclThunk(d *ast.FuncDecl, sym *analyzer.FuncSymbol, callee string) {
	name, has := g.asmName(d.Attrs, attrCDecl)
	if !has || name == "" {
		return
	}
	sig := sym.Signature()
	if !cCompatible(sig) {
		g.errorAt(d, "cannot lower '@_cdecl' for this signature yet: a C entry "+
			"point takes and answers values that cross in a register, and "+
			"this one does not")
		return
	}
	if existing := g.m.Lookup(name); existing != nil && !existing.IsDeclaration() {
		g.errorAt(d, "'@_cdecl(\""+name+"\")' names '"+name+"', which this "+
			"module already defines")
		return
	}

	saved, savedBlk, savedEntry := g.fn, g.blk, g.entry
	defer func() { g.fn, g.blk, g.entry = saved, savedBlk, savedEntry }()

	f := g.m.Func(name).SetSourceName(sym.Name()).
		SetLinkage(sil.Public).SetAttr("ossa").SetAttr("thunk")
	f.Type().Convention = sil.C
	g.fn, g.entry = f, false

	args := make([]*sil.Value, 0, len(sig.Params))
	for _, p := range sig.Params {
		t := lowerType(p.BodyType())
		args = append(args, f.Param(t, sil.ParamUnowned))
	}
	g.blk = f.Entry()

	target := g.m.Func(callee).SetSourceName(sym.Name())
	if g.needsType(target) {
		g.declare(target, sym)
	}
	ref := g.blk.FunctionRef(target)

	if sig.Results == nil || isVoid(sig.Results) {
		g.blk.Apply(ref, sil.Type{}, args...)
		g.blk.Return(g.blk.Tuple(sil.Object(&types.Tuple{}), nil))
		return
	}
	result := lowerType(sig.Results)
	f.SetResult(result, resultConvention(result))
	g.blk.Return(g.blk.Apply(ref, result, args...))
}

// cCompatible reports whether a signature contains only trivial register-passable types for C interop.
func cCompatible(sig *types.Signature) bool {
	if sig == nil || sig.Throws || len(sig.TypeParams) > 0 {
		return false
	}
	for _, p := range sig.Params {
		t := lowerType(p.BodyType())
		if !t.IsValid() || t.IsAddress() || !t.Trivial() ||
			byAddress(paramConvention(p, t)) {
			return false
		}
	}
	if sig.Results != nil && !isVoid(sig.Results) {
		t := lowerType(sig.Results)
		if !t.IsValid() || t.IsAddress() || !t.Trivial() {
			return false
		}
	}
	return true
}

// fileOf is the file of this module a node was parsed from, found by the
// node itself. A position counts from the start of its own file, so every
// file has the same ones, and a position alone cannot say which.
func (g *gen) fileOf(n ast.Node) *token.File {
	if n == nil || g.fileIdx == nil {
		return nil
	}
	return g.fileIdx.of(n)
}

// A fileIndex answers which file of a module a node is in, for every gen
// of the module at once: each file's gen asks, and the index is built
// once, the first time any of them does.
//
// It replaces a walk per question. A node from the core or an import was
// looked for in the module's own files first, walked to the end of each,
// once per function the module named.
//
// Two tiers. Almost every question is about a declaration, so the first
// is every node outside a function body -- declarations, their
// parameters and defaults, their members -- which is a small part of an
// imported file. Only a node inside a body (a local function) walks the
// rest, once. The core's own declarations are in neither file list and
// are answered "no file" by the first tier without walking anything.
type fileIndex struct {
	files   []*ast.File
	shallow map[ast.Node]*token.File
	deep    map[ast.Node]*token.File
}

func (x *fileIndex) of(n ast.Node) *token.File {
	if x.shallow == nil {
		x.shallow = make(map[ast.Node]*token.File, 1024)
		for _, f := range x.files {
			if f != nil && f.Unit != nil {
				index(x.shallow, f, f.Unit, false)
			}
		}
		if core, _, _ := core.Files(); core != nil {
			index(x.shallow, core, nil, false)
		}
	}
	if f, ok := x.shallow[n]; ok {
		return f
	}
	if x.deep == nil {
		x.deep = make(map[ast.Node]*token.File, 4096)
		for _, f := range x.files {
			if f != nil && f.Unit != nil {
				index(x.deep, f, f.Unit, true)
			}
		}
	}
	return x.deep[n]
}

// index records unit as the file of every node in f -- the first file to
// claim a node keeps it -- going into function bodies only when deep.
func index(m map[ast.Node]*token.File, f *ast.File, unit *token.File, deep bool) {
	ast.Inspect(f, func(n ast.Node) bool {
		if n == nil {
			return true
		}
		if _, dup := m[n]; !dup {
			m[n] = unit
		}
		if _, body := n.(*ast.CodeBlock); body && !deep {
			return false
		}
		return true
	})
}

// builtinOp is the instruction `@_builtin` makes a core function, if it
// is one. Only the core declares these.
func (g *gen) builtinOp(sym *analyzer.FuncSymbol) (string, bool) {
	if sym == nil || g.info.Imported[sym] != "Swift" {
		return "", false
	}
	_, file, _ := core.Files()
	return g.asmNameIn(file, funcAttrs(sym), attrBuiltin)
}

// callBuiltinOp lowers a call of a core function declared @_builtin: the
// arguments' machine values, the instruction, and its result wrapped as
// the function's result type -- which is the first operand's type, as it
// is for Swift's Builtin.int_ctpop and Builtin.int_sqrt.
func (g *gen) callBuiltinOp(e *ast.CallExpr, sym *analyzer.FuncSymbol, op string) *sil.Value {
	sig := sym.Signature()
	if e.Args == nil || len(e.Args.Args) != len(sig.Params) || len(sig.Params) == 0 {
		g.refuse(e, "a builtin called with other than its operands")
		return nil
	}
	machine, ok := builtinMachine(sig.Params[0].Type)
	if !ok {
		g.refuse(e, "a builtin on a type that is not a number or a metatype")
		return nil
	}
	raws := make([]*sil.Value, len(e.Args.Args))
	for i, a := range e.Args.Args {
		v := g.rvalue(a.X)
		if v == nil {
			return nil
		}
		raws[i] = g.machine(v, sig.Params[i].Type)
	}
	res := sig.Results
	// An arithmetic instruction that reports overflow answers the value
	// and a flag: (T, Bool), from SIL's (Builtin.IntN, Builtin.Int1).
	if tt, ok := res.Underlying().(*types.Tuple); ok && len(tt.Elements) == 2 {
		val, flag := tt.Elements[0].Type, tt.Elements[1].Type
		bit := sil.Object(sil.BuiltinInt1)
		pair := sil.Object(&types.Tuple{Elements: []*types.TupleElement{
			{Type: builtinFor(val)}, {Type: sil.BuiltinInt1},
		}})
		raws = append(raws, g.blk.IntegerLiteral(bit, -1))
		both := g.blk.Builtin(op+"_"+machine, pair, raws...)
		v := g.blk.Struct(lowerType(val), g.blk.TupleExtract(both, 0, sil.Object(builtinFor(val))))
		f := g.blk.Struct(lowerType(flag), g.blk.TupleExtract(both, 1, bit))
		return g.blk.Tuple(lowerType(res), v, f)
	}
	name := op + "_" + machine
	// A conversion names both types, as SIL's do: bitcast_FPIEEE64_Int64.
	// (A comparison's result is of another type too, but it is named for
	// its operands alone: cmp_eq_RawPointer.)
	if to, ok := builtinMachine(res); ok && to != machine && op == "bitcast" {
		name += "_" + to
	}
	out := g.blk.Builtin(name, sil.Object(builtinFor(res)), raws...)
	return g.blk.Struct(lowerType(res), out)
}

// builtinMachine is the machine type SIL names an instruction on t by: a
// number's own, and a raw pointer for a metatype, which is its metadata.
func builtinMachine(t types.Type) (string, bool) {
	if _, ok := t.(*types.Metatype); ok {
		return "RawPointer", true
	}
	_, machine, ok := core.Layout(t)
	return machine, ok
}
