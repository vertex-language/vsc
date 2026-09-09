package gen

import (
	"github.com/vertex-language/vsc/analyzer"
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/token"
	"github.com/vertex-language/vsc/types"
	"github.com/vertex-language/vsc/vil"
)

// Naming a symbol across the language boundary.
//
// Two attributes decide what a declaration is called in the object
// file rather than what this compiler would call it, and they are the
// two halves of C interoperability that need no importer: one names a
// symbol somebody else defined, and the other defines a symbol
// somebody else can call.
//
// `@_silgen_name("abs")` says this declaration *is* that symbol. With
// no body it is a declaration of it, which is how a C function is
// called without a header to import:
//
//	sil hidden_external @abs : $@convention(thin) (Int32) -> Int32
//
// It keeps the *Vertex* calling convention, which is the footgun the
// underscore is warning about: it happens to be right for the scalars
// C and Swift agree on, and is wrong the moment a parameter is not
// one. swiftc writes @convention(thin) here too, for the same reason.
//
// `@_cdecl("vs_add")` is the other direction and is not a rename. The
// function stays what it was, under the name it had, and a second
// function appears beside it: a thunk with the C convention that
// calls it. That is what swiftc emits, and it is why a `@_cdecl`
// function is still callable from Vertex by its own name.
//
//	sil [thunk] @vs_add : $@convention(c) (Int32, Int32) -> Int32
//
// SIL names that thunk with the mangled name plus `To` and carries
// the C name in an `[asmname "vs_add"]` attribute. VIL has one name
// per function and no asmname, so the thunk is named for the symbol
// directly -- the object file is the same either way, and the symbol
// is the whole point of the attribute.

const (
	attrSilgenName = "_silgen_name"
	attrCDecl      = "_cdecl"
	attrObjC       = "objc"
)

// asmName is the symbol an attribute names, and reports whether the
// declaration carried that attribute at all.
//
// An attribute's arguments are kept as the tokens they were written
// as -- the grammar says BalancedTokens and the parser means it --
// so the string is read back out of the source here rather than from
// a parsed argument list. One string literal is the whole of what
// either of these attributes takes.
func (g *gen) asmName(attrs []*ast.Attr, want string) (string, bool) {
	for _, a := range attrs {
		if a == nil || attrNameOf(g, a) != want {
			continue
		}
		// A string is scanned as its parts -- a quote, the segments
		// between the interpolations, and a quote -- so the symbol is
		// the one segment. An attribute whose argument interpolates
		// has more than one and is not a symbol at all, which the
		// caller reports as a missing name.
		var segments []string
		for _, t := range a.Tokens {
			if t.Kind == token.STRING_SEGMENT {
				segments = append(segments, string(g.file.Slice(t.Pos, t.End)))
			}
		}
		if len(segments) == 1 && segments[0] != "" {
			return segments[0], true
		}
		// The attribute is there and the symbol is not, which is a
		// declaration asking for a name it did not give. Reported by
		// the caller, which knows which attribute it was.
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

// funcAttrs is the attributes a function symbol's declaration carries.
//
// It goes back through the symbol because a call has the symbol and
// not the syntax: what a function is called has to be the same answer
// at the definition and at every use, and only the declaration says
// what that is.
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
	return g.asmName(funcAttrs(sym), attrSilgenName)
}

// interopAttrs checks the attributes on a function declaration, and
// reports whether lowering may go on.
//
// The two this compiler honours are checked for the name they must
// carry. The rest are refused: an attribute whose whole purpose is a
// symbol or a dispatch that does not appear is worse than one that is
// missing, because the source says a thing happened and the program
// does not do it. `@objc` is that -- it needs the Objective-C runtime
// and there is none here -- and so is any spelling of these two
// without a name.
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

// cdeclThunk emits the C-callable function `@_cdecl` asks for: a
// wrapper with the platform's convention that calls the one the
// source wrote.
//
// A thunk rather than a rename, because both names are real. Vertex
// calls the function by its own symbol and C calls it by this one,
// and the two are separate entry points to the same body.
//
// Only the scalars survive the boundary today. A parameter that
// crosses by address, or that owns what it holds, is a C signature
// this compiler cannot claim to have written -- the convention says
// where the bytes are, and there is nothing here yet that says what
// the C side would do with them.
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
	// A name somebody else already defined, in either order: the
	// thunk would be appended to that function's blocks and the
	// result is one symbol with two bodies. The other order -- the
	// thunk first and the collision after -- is caught where the
	// definition is built, because only one of the two can be here
	// yet.
	if existing := g.m.Lookup(name); existing != nil && !existing.IsDeclaration() {
		g.errorAt(d, "'@_cdecl(\""+name+"\")' names '"+name+"', which this "+
			"module already defines")
		return
	}

	saved, savedBlk, savedEntry := g.fn, g.blk, g.entry
	defer func() { g.fn, g.blk, g.entry = saved, savedBlk, savedEntry }()

	f := g.m.Func(name).SetSourceName(sym.Name()).
		SetLinkage(vil.Public).SetAttr("ossa").SetAttr("thunk")
	f.Type().Convention = vil.C
	g.fn, g.entry = f, false

	args := make([]*vil.Value, 0, len(sig.Params))
	for _, p := range sig.Params {
		t := lowerType(p.BodyType())
		args = append(args, f.Param(t, vil.ParamUnowned))
	}
	g.blk = f.Entry()

	target := g.m.Func(callee).SetSourceName(sym.Name())
	if g.needsType(target) {
		g.declare(target, sym)
	}
	ref := g.blk.FunctionRef(target)

	if sig.Results == nil || isVoid(sig.Results) {
		g.blk.Apply(ref, vil.Type{}, args...)
		g.blk.Return(g.blk.Tuple(vil.Object(&types.Tuple{}), nil))
		return
	}
	result := lowerType(sig.Results)
	f.SetResult(result, resultConvention(result))
	g.blk.Return(g.blk.Apply(ref, result, args...))
}

// cCompatible reports whether a signature is one this compiler can
// give a C entry point.
//
// Every parameter and the result must cross in a register and own
// nothing: those are the values whose place in the ABI is the same
// question for both languages. Anything else -- an address, a
// reference, a String -- is a claim about what C does with it, and
// this makes no such claim.
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
