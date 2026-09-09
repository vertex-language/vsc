package gen

import (
	"strings"

	"github.com/vertex-language/vsc/analyzer"
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/mangle"
	"github.com/vertex-language/vsc/types"
	"github.com/vertex-language/vsc/vil"
)

// Initializers a type declares for itself.
//
// A struct that declares none gets the memberwise one for free, and
// construct() emits that in place -- there is nothing to call. One
// that declares its own is a function like any other, and swiftc
// shows its shape:
//
//	sil @P.init(v:) : $@convention(method) (Int32, @thin P.Type) -> P {
//	bb0(%0 : $Int32, %1 : $@thin P.Type):
//	  %2 = alloc_box ${ var P }, var, name "self"
//	  %3 = mark_uninitialized [rootself] %2
//	  %4 = begin_borrow [var_decl] %3
//	  %5 = project_box %4, 0
//	  … struct_element_addr %5, #P.x ; assign …
//	  %22 = load [trivial] %5
//	  return %22
//	}
//
// So self is a var of the type being made, uninitialized until the
// body has filled it in, and the value returned is what the body
// left there. The metatype goes last, where the method convention
// puts a receiver -- an initializer is a method on the type rather
// than on an instance.
//
// A class is refused. Its initializer is two functions rather than
// one, an allocating entry point and an initializing one, and a
// subclass's has to reach its superclass's; none of that is written.

// initializer lowers one `init` declaration.
func (g *gen) initializer(d *ast.InitDecl, recv types.Type) {
	if d.Body == nil {
		return
	}
	switch {
	case d.Question.IsValid() || d.Exclaim.IsValid():
		g.refuse(d, "a failable initializer")
		return
	case isClass(recv):
		g.classInitializer(d, recv)
		return
	case !isStructType(recv):
		g.refuse(d, "an initializer this type declares")
		return
	}
	sig := g.initSignature(d, recv)
	if sig == nil {
		g.refuse(d, "this initializer")
		return
	}

	name := g.initSymbol(recv, sig)
	if name == "" {
		g.refuse(d, "this initializer")
		return
	}
	f := g.m.Func(name).SetSourceName("init").SetAttr("ossa")

	outer := struct {
		fn      *vil.Func
		entry   bool
		blk     *vil.Block
		scopes  []*scope
		locals  map[analyzer.Symbol]*local
		loops   []loop
		pending string
		recv    types.Type
		self    *local
		initRet func()
	}{g.fn, g.entry, g.blk, g.scopes, g.locals, g.loops, g.pending, g.recv, g.self, g.initReturn}
	defer func() {
		g.fn, g.entry, g.blk = outer.fn, outer.entry, outer.blk
		g.scopes, g.locals = outer.scopes, outer.locals
		g.loops, g.pending = outer.loops, outer.pending
		g.recv, g.self = outer.recv, outer.self
		g.initReturn = outer.initRet
	}()

	g.fn = f
	f.Type().Params = nil
	g.entry = false
	g.locals = map[analyzer.Symbol]*local{}
	g.scopes = nil
	g.loops, g.pending = nil, ""
	g.recv = recv
	g.push()
	g.blk = f.Entry()

	params := g.initParams(d)
	for i, p := range sig.Params {
		t := lowerType(p.Type)
		v := f.Param(t, paramConvention(p, t))
		if i < len(params) && params[i] != nil {
			g.locals[params[i]] = &local{value: v, typ: t}
		}
		if p.Name != "" {
			g.blk.DebugValue(v, p.Name, "let", "argno "+itoa(i+1))
		}
		g.destroyLater(v)
	}
	// The metatype, last, where a method's receiver goes. Nothing
	// reads it -- a struct's layout is known statically -- but the
	// convention says it is passed, and a caller passes it.
	mt := vil.ThinMetatype(recv)
	f.Param(mt, vil.ParamUnowned)
	f.Type().Convention = vil.Method
	f.SetResult(lowerType(recv), resultConvention(lowerType(recv)))

	// self: a var of the type being made, which the body writes into
	// and which nothing may read until it has.
	t := lowerType(recv)
	box := g.blk.AllocBox(t, "self", "var")
	marked := g.blk.MarkUninitialized(box, "rootself")
	borrow := g.blk.BeginBorrow(marked, "var_decl")
	addr := g.blk.ProjectBox(borrow, 0, t)
	g.self = &local{addr: addr, box: marked, typ: t}

	// What a return from this initializer emits, wherever it is
	// written: the value built so far, with the box torn down. A bare
	// `return` in the body leaves early with the same thing the end
	// of the body produces.
	g.initReturn = func() {
		v := g.blk.Load(addr, loadQualifier(t))
		g.blk.EndBorrow(borrow)
		g.blk.DestroyValue(marked)
		g.unwind()
		g.blk.Return(v)
	}

	g.block(d.Body)

	if g.blk != nil && g.blk.Term() == nil {
		g.initReturn()
	}
}

// classInitializer lowers one `init` a class declares.
//
// A class's initializer is two functions where a struct's is one,
// which is what Swift's own `fC` and `fc` say: the allocating entry
// point makes the instance and the initializing one fills it in. They
// are separate because a subclass's initializer runs its
// superclass's on the same instance -- one allocation, two bodies --
// and separating them is what makes that possible.
//
// So both are emitted, under swiftc's two names. What differs from
// swiftc is the receiver of the allocating one. A class declared in
// another module is made by reading the instance's size out of the
// metadata, which is why calling one takes a real metatype; a class
// declared here is allocated by this compiler's own alloc_ref, whose
// size is known where it is written, so its metatype carries nothing.
//
// A class with a superclass is refused. Its initializer has to reach
// the one above it, and `super.init` is not lowered.
func (g *gen) classInitializer(d *ast.InitDecl, recv types.Type) {
	cl, ok := recv.Underlying().(*types.Class)
	if !ok {
		return
	}
	if cl.Superclass != nil {
		g.refuse(d, "an initializer of a class with a superclass, which has to run "+
			"the one above it")
		return
	}
	sig := g.classInitSignature(d, recv, cl)
	if sig == nil {
		g.refuse(d, "this initializer")
		return
	}
	alloc := g.initSymbol(recv, sig)
	if alloc == "" || !strings.HasSuffix(alloc, "fC") {
		g.refuse(d, "this initializer")
		return
	}
	// swiftc's two names differ in one letter: `fC` allocates and
	// `fc` fills in.
	initializing := alloc[:len(alloc)-1] + "c"

	g.classInitBody(d, recv, sig, initializing)
	g.classAllocator(recv, sig, alloc, initializing)
}

// classInitBody emits the entry point that fills an instance in: the
// declared parameters, then the instance itself as the receiver.
func (g *gen) classInitBody(d *ast.InitDecl, recv types.Type,
	sig *types.Signature, name string) {
	f := g.m.Func(name).SetSourceName("init").SetAttr("ossa")
	restore := g.saveFunction()
	defer restore()

	g.fn, g.entry = f, false
	f.Type().Params = nil
	g.locals = map[analyzer.Symbol]*local{}
	g.scopes, g.loops, g.pending = nil, nil, ""
	g.recv, g.self, g.initReturn = recv, nil, nil
	g.push()
	g.blk = f.Entry()

	params := g.initParams(d)
	for i, p := range sig.Params {
		t := lowerType(p.Type)
		v := f.Param(t, paramConvention(p, t))
		if i < len(params) && params[i] != nil {
			g.locals[params[i]] = &local{value: v, typ: t}
		}
		if p.Name != "" {
			g.blk.DebugValue(v, p.Name, "let", "argno "+itoa(i+1))
		}
		g.destroyLater(v)
	}
	// self, last, where a method's receiver goes. It is the instance
	// rather than storage holding one: the allocation happened in the
	// entry point above, and what this body does is write through the
	// reference.
	t := lowerType(recv)
	self := f.Param(t, vil.ParamGuaranteed)
	f.Type().Convention = vil.Method
	f.SetResult(t, resultConvention(t))
	g.blk.DebugValue(self, "self", "let")

	// What a return from this initializer emits. A class's is the
	// instance it was handed rather than a box it filled, but a bare
	// `return` in the body means the same thing it does in a struct's:
	// leave early with what has been made.
	g.initReturn = func() {
		g.unwind()
		g.blk.Return(g.blk.CopyValue(self))
	}

	g.block(d.Body)

	if g.blk != nil && g.blk.Term() == nil {
		g.initReturn()
	}
}

// classAllocator emits the entry point that makes the instance and
// hands it to the one that fills it in.
func (g *gen) classAllocator(recv types.Type, sig *types.Signature, name, body string) {
	f := g.m.Func(name).SetSourceName("init").SetAttr("ossa")
	restore := g.saveFunction()
	defer restore()

	g.fn, g.entry = f, false
	f.Type().Params = nil
	g.locals = map[analyzer.Symbol]*local{}
	g.scopes, g.loops, g.pending = nil, nil, ""
	g.recv, g.self = recv, nil
	g.push()
	g.blk = f.Entry()

	t := lowerType(recv)
	var args []*vil.Value
	for _, p := range sig.Params {
		pt := lowerType(p.Type)
		args = append(args, f.Param(pt, paramConvention(p, pt)))
	}
	f.Param(vil.ThinMetatype(recv), vil.ParamUnowned)
	f.Type().Convention = vil.Method
	f.SetResult(t, resultConvention(t))

	callee := g.m.Func(body).SetSourceName("init")
	if g.needsType(callee) {
		for _, p := range sig.Params {
			pt := lowerType(p.Type)
			callee.Type().Params = append(callee.Type().Params,
				vil.Param{Type: pt, Convention: paramConvention(p, pt)})
		}
		callee.Type().Params = append(callee.Type().Params,
			vil.Param{Type: t, Convention: vil.ParamGuaranteed})
		callee.Type().Convention = vil.Method
		callee.SetResult(t, resultConvention(t))
	}
	obj := g.blk.AllocRef(t)
	args = append(args, obj)
	made := g.blk.Apply(g.blk.FunctionRef(callee), t, args...)
	g.blk.DestroyValue(obj)
	g.blk.Return(made)
}

// classInitSignature is the initializer's type among the ones the
// class declared.
func (g *gen) classInitSignature(d *ast.InitDecl, recv types.Type,
	cl *types.Class) *types.Signature {
	for _, cand := range cl.Inits {
		if cand != nil && sameInitParams(cand, d) {
			out := *cand
			out.Results = recv
			return &out
		}
	}
	return nil
}

// saveFunction remembers what the generator was in the middle of, so
// that emitting a function inside another one puts everything back.
func (g *gen) saveFunction() func() {
	fn, entry, blk := g.fn, g.entry, g.blk
	scopes, locals := g.scopes, g.locals
	loops, pending := g.loops, g.pending
	recv, self := g.recv, g.self
	initRet := g.initReturn
	return func() {
		g.fn, g.entry, g.blk = fn, entry, blk
		g.scopes, g.locals = scopes, locals
		g.loops, g.pending = loops, pending
		g.recv, g.self = recv, self
		g.initReturn = initRet
	}
}

// initSignature is the initializer's type: what it was declared with,
// returning the type it makes.
func (g *gen) initSignature(d *ast.InitDecl, recv types.Type) *types.Signature {
	st, ok := recv.Underlying().(*types.Struct)
	if !ok {
		return nil
	}
	for _, cand := range st.Inits {
		if cand != nil && sameInitParams(cand, d) {
			out := *cand
			out.Results = recv
			return &out
		}
	}
	return nil
}

// sameInitParams reports whether a recorded signature is the one this
// declaration wrote. A type may declare several initializers, and
// they are told apart by their parameters the way overloads are.
func sameInitParams(sig *types.Signature, d *ast.InitDecl) bool {
	var n int
	if d.Sig != nil {
		n = len(d.Sig.Params)
	}
	return len(sig.Params) == n
}

// initParams is the symbol each parameter binds.
//
// The analyzer records them in the scope it checked the body in
// rather than against the syntax, and by name -- so they are looked
// up there, in order, the way a function's are.
func (g *gen) initParams(d *ast.InitDecl) []analyzer.Symbol {
	if d.Sig == nil {
		return nil
	}
	out := make([]analyzer.Symbol, len(d.Sig.Params))
	scope := g.info.Scopes[d]
	if scope == nil {
		return out
	}
	for i, p := range d.Sig.Params {
		name := p.Name
		if name == nil {
			name = p.Label
		}
		if name == nil {
			continue
		}
		if sym := scope.Lookup(g.text(name)); sym != nil {
			out[i] = sym
		}
	}
	return out
}

// classMetatype is the receiver a class's allocating initializer
// takes: its metadata, fetched at run time from the accessor the
// module that declared the class exports.
func (g *gen) classMetatype(t types.Type) (*vil.Value, bool) {
	d := mangle.Decl{
		Module:   g.moduleOfType(t),
		Context:  nominalChain(t),
		ModuleOf: g.moduleOfType,
	}
	sym, err := mangle.MetadataAccessor(d)
	if err != nil {
		return nil, false
	}
	return g.blk.ClassMetatype(lowerType(t), sym), true
}

// initSymbol is the name an initializer is given.
func (g *gen) initSymbol(recv types.Type, sig *types.Signature) string {
	d := mangle.Decl{
		Module:    g.moduleOfType(recv),
		Signature: sig,
		ModuleOf:  g.moduleOfType,
	}
	d.Context = nominalChain(recv)
	name, err := mangle.Initializer(d)
	if err != nil {
		return ""
	}
	return name
}

func isStructType(t types.Type) bool {
	if t == nil {
		return false
	}
	_, ok := t.Underlying().(*types.Struct)
	return ok
}

// callInit calls an initializer the type declared.
//
// The arguments the program wrote, then the metatype -- the shape a
// method call has, because that is what an initializer is: a method
// on the type. Which of several initializers is a question about the
// arguments, and is answered the way an overload is.
// callClassInit calls a class's allocating initializer, which is
// where an imported class is made: the body is in the module that
// declared it, and what this side supplies is the arguments and the
// metadata.
func (g *gen) callClassInit(e *ast.CallExpr, t types.Type, cl *types.Class) *vil.Value {
	// A subclass's initializer has to run the one above it on the
	// same instance, and `super.init` is not lowered -- so the body
	// was refused where it was written, and calling it would name a
	// symbol nothing defines. See classInitializer.
	if cl.Superclass != nil && !g.importedType(t) {
		g.refuse(e, "an initializer of a class with a superclass, which has to run "+
			"the one above it")
		return nil
	}
	var args []*ast.CallArg
	if e.Args != nil {
		args = e.Args.Args
	}
	sig := pickInitFrom(cl.Inits, args)
	if sig == nil {
		g.refuse(e, "a call to an initializer whose arguments do not match one the class declares")
		return nil
	}
	out := *sig
	out.Results = t
	return g.applyInit(e, t, &out, args)
}

func (g *gen) callInit(e *ast.CallExpr, t types.Type, st *types.Struct) *vil.Value {
	var args []*ast.CallArg
	if e.Args != nil {
		args = e.Args.Args
	}
	sig := pickInit(st, args)
	if sig == nil {
		g.refuse(e, "a call to an initializer whose arguments do not match one it declares")
		return nil
	}
	out := *sig
	out.Results = t
	return g.applyInit(e, t, &out, args)
}

// importedType reports whether a type was declared in another module.
func (g *gen) importedType(t types.Type) bool {
	if m := g.moduleOfType(t); m != "" && m != g.module {
		return true
	}
	return false
}

// applyInit emits the call to an initializer this side does not
// define.
func (g *gen) applyInit(e *ast.CallExpr, t types.Type, out *types.Signature, args []*ast.CallArg) *vil.Value {
	name := g.initSymbol(t, out)
	if name == "" {
		g.refuse(e, "this initializer")
		return nil
	}
	callee := g.m.Func(name).SetSourceName("init")
	if g.needsType(callee) {
		for _, p := range out.Params {
			pt := lowerType(p.Type)
			callee.Type().Params = append(callee.Type().Params,
				vil.Param{Type: pt, Convention: paramConvention(p, pt)})
		}
		self := vil.ThinMetatype(t)
		if isClass(t) && g.importedType(t) {
			self = vil.ThickMetatype(t)
		}
		callee.Type().Params = append(callee.Type().Params,
			vil.Param{Type: self, Convention: vil.ParamUnowned})
		callee.Type().Convention = vil.Method
		callee.SetResult(lowerType(t), resultConvention(lowerType(t)))
	}
	ref := g.blk.FunctionRef(callee)

	// Use g.arguments to evaluate call arguments, which respects each
	// parameter's calling convention. The previous g.rvalue loop called
	// g.forget on every owned argument, which dropped the cleanup for
	// values that are only borrowed (@guaranteed) by the callee — causing
	// a leak. g.arguments uses g.expr (leaving cleanups in place for
	// @guaranteed parameters) and only strips cleanups for @owned/@in ones
	// via ConsumesArgument, matching the same approach callFunc uses.
	vals, ok := g.arguments(e, out)
	if !ok {
		return nil
	}
	// The receiver. A struct's metatype is thin -- the type is known
	// and there is nothing to carry -- while a class's is its
	// metadata, which the initializer reads the instance's size out
	// of and which has to be asked for.
	// A class declared elsewhere is made through its own allocating
	// initializer, which reads the instance's size out of the
	// metadata -- so the metadata is asked for. A class declared here
	// is allocated by this compiler's own alloc_ref, whose size is
	// known statically, so its metatype carries nothing either.
	if isClass(t) && g.importedType(t) {
		meta, ok := g.classMetatype(t)
		if !ok {
			g.refuse(e, "an initializer of a class whose metadata cannot be named")
			return nil
		}
		vals = append(vals, meta)
	} else {
		vals = append(vals, g.blk.Metatype(lowerType(t)))
	}

	v := g.blk.Apply(ref, lowerType(t), vals...)
	g.destroyLater(v)
	return v
}

// pickInit is the initializer a call's arguments name.
func pickInit(st *types.Struct, args []*ast.CallArg) *types.Signature {
	return pickInitFrom(st.Inits, args)
}

// pickInitFrom is pickInit over a list, which is what a class needs:
// the same question, asked of a type that is not a struct.
func pickInitFrom(inits []*types.Signature, args []*ast.CallArg) *types.Signature {
	for _, sig := range inits {
		if sig != nil && len(sig.Params) == len(args) {
			return sig
		}
	}
	return nil
}
