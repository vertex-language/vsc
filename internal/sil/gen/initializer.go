package gen

import (
	"strings"

	"github.com/vertex-language/vsc/analyzer"
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/internal/sil"
	"github.com/vertex-language/vsc/mangle"
	"github.com/vertex-language/vsc/types"
)

// initializer lowers one `init` declaration.
func (g *gen) initializer(d *ast.InitDecl, recv types.Type) {
	if d.Body == nil {
		return
	}
	switch {
	case (d.Question.IsValid() || d.Exclaim.IsValid()) && isClass(recv):
		g.refuse(d, "a failable initializer of a class")
		return
	case isClass(recv):
		g.classInitializer(d, recv)
		return
	case !isStructType(recv) && !isBasicValue(recv) && !isEnumType(recv):
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
	g.structInitBody(d, recv, sig, name)
}

// structInitBody emits a struct initializer's body under this symbol:
// the declaration's own, or one specialized for a generic struct's
// instance.
func (g *gen) structInitBody(d *ast.InitDecl, recv types.Type, sig *types.Signature, name string) {
	f := g.m.Func(name).SetSourceName("init").SetLinkage(g.initLinkage()).SetAttr("ossa")

	outer := struct {
		fn       *sil.Func
		entry    bool
		blk      *sil.Block
		scopes   []*scope
		locals   map[analyzer.Symbol]*local
		loops    []loop
		pending  string
		recv     types.Type
		self     *local
		initRet  func()
		initFail func()
		throws   bool
		catches  []catchTarget
	}{g.fn, g.entry, g.blk, g.scopes, g.locals, g.loops, g.pending, g.recv, g.self, g.initReturn, g.initFail, g.throws, g.catches}
	defer func() {
		g.fn, g.entry, g.blk = outer.fn, outer.entry, outer.blk
		g.scopes, g.locals = outer.scopes, outer.locals
		g.loops, g.pending = outer.loops, outer.pending
		g.recv, g.self = outer.recv, outer.self
		g.initReturn, g.initFail = outer.initRet, outer.initFail
		g.throws, g.catches = outer.throws, outer.catches
	}()

	g.fn = f
	f.Type().Params = nil
	g.entry = false
	g.locals = map[analyzer.Symbol]*local{}
	g.scopes = nil
	g.loops, g.pending = nil, ""
	// An initializer declared 'throws' can throw before it finishes filling
	// self in; its error result is wired like any throwing function's.
	g.throws, g.catches = sig.Throws, nil
	if sig.Throws {
		f.SetThrows(sil.Object(sil.BuiltinNativeObj))
	}
	g.recv = recv
	g.push()
	g.blk = f.Entry()

	params := g.initParams(d)
	for i, p := range sig.Params {
		t := lowerType(p.BodyType())
		v := f.Param(t, paramConvention(p, t))
		if i < len(params) && params[i] != nil {
			g.locals[params[i]] = &local{value: v, typ: t}
		}
		if p.Name != "" {
			g.blk.DebugValue(v, p.Name, "let", "argno "+itoa(i+1))
		}
		g.destroyLater(v)
	}
	mt := sil.ThinMetatype(recv)
	f.Param(mt, sil.ParamUnowned)
	f.Type().Convention = sil.Method
	// An init? makes an optional: .some(self) where the body finishes,
	// .none where it returns nil.
	made := types.Type(recv)
	if sig.Failable {
		made = &types.Optional{Wrapped: recv}
	}
	f.SetResult(lowerType(made), resultConvention(lowerType(made)))

	t := lowerType(recv)
	// A property's first assignment in the body lets go of what was there
	// before, which for one with no default is nothing: storage that
	// starts as zero holds nothing to let go of.
	selfAttrs := []string{"var"}
	if !t.Trivial() {
		selfAttrs = append(selfAttrs, "zeroed")
	}
	box := g.blk.AllocBox(t, "self", selfAttrs...)
	marked := g.blk.MarkUninitialized(box, "rootself")
	borrow := g.blk.BeginBorrow(marked, "var_decl")
	addr := g.blk.ProjectBox(borrow, 0, t)
	g.self = &local{addr: addr, box: marked, typ: t}

	// Register the self box's teardown as scope cleanups so that every path
	// out -- falling off the end, an explicit return, and a `throw` before
	// self is finished -- ends the borrow and frees the box. Cleanups run
	// last-registered-first, so destroy is registered before end_borrow to
	// have the borrow end before the box it borrows is destroyed. A throwing
	// initializer relies on this on its error path.
	selfCleanup := len(g.top().cleanups)
	g.top().cleanups = append(g.top().cleanups, cleanup{destroy: marked})
	g.top().cleanups = append(g.top().cleanups, cleanup{endBorrow: borrow})

	// A stored property with a default value has it before the body runs,
	// as Swift gives it; an assignment in the body replaces it. Without
	// this a property the body left alone was whatever the memory held.
	if st, ok := recv.Underlying().(*types.Struct); ok {
		for _, fld := range st.Fields {
			if fld == nil || !fld.HasDefault {
				continue
			}
			def := g.info.FieldDefaults[fld]
			if def == nil {
				// An instance of a generic struct: the default is recorded
				// against the declaration's property.
				def, _ = g.instanceDefault(recv, fld.Name)
			}
			if def == nil {
				continue
			}
			v := g.rvalue(def)
			if v == nil {
				continue
			}
			ft := lowerType(fld.Type)
			v = g.optionalFor(def, v, g.typeOf(def), fld.Type)
			at := g.blk.StructElementAddr(addr, memberName(recv, fld.Name), ft.Address())
			g.blk.Store(v, at, storeQualifier(ft))
		}
	}

	g.initReturn = func() {
		// Load self out as an owned copy before the box cleanups run, so it
		// survives the box being destroyed, then let unwind emit the borrow
		// end and box destroy (registered above) along with any parameters.
		v := g.blk.Load(addr, loadQualifier(t))
		g.unwind()
		if sig.Failable {
			v = g.blk.Enum(lowerType(made), optionalSome, v)
		}
		g.blk.Return(v)
	}
	g.initFail = nil
	if sig.Failable {
		g.initFail = func() {
			g.unwind()
			g.blk.Return(g.blk.Enum(lowerType(made), optionalNone, nil))
		}
	}
	_ = selfCleanup

	g.block(d.Body)

	if g.blk != nil && g.blk.Term() == nil {
		g.initReturn()
	}
}

// classInitializer lowers a class initializer into allocating (fC) and initializing (fc) functions.
func (g *gen) classInitializer(d *ast.InitDecl, recv types.Type) {
	cl, ok := recv.Underlying().(*types.Class)
	if !ok {
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
	if sig.Convenience {
		g.convenienceInit(d, recv, sig, alloc)
		return
	}
	initializing := alloc[:len(alloc)-1] + "c"

	g.classInitBody(d, recv, sig, initializing)
	g.classAllocator(recv, sig, alloc, initializing)
}

// convenienceInit emits a class's convenience initializer: an entry that
// makes the instance by handing on to another of the class's initializers
// -- its `self.init(...)` -- and then runs the rest of its body on it.
func (g *gen) convenienceInit(d *ast.InitDecl, recv types.Type, sig *types.Signature, name string) {
	f := g.m.Func(name).SetSourceName("init").SetLinkage(g.initLinkage()).SetAttr("ossa")
	restore := g.saveFunction()
	convenience, convSelf := g.convenience, g.convSelf
	defer func() {
		restore()
		g.convenience, g.convSelf = convenience, convSelf
	}()
	g.fn, g.entry = f, false
	f.Type().Params = nil
	g.locals = map[analyzer.Symbol]*local{}
	g.scopes, g.loops, g.pending = nil, nil, ""
	g.recv, g.self, g.initReturn = recv, nil, nil
	g.convenience, g.convSelf = true, nil
	g.push()
	g.blk = f.Entry()

	params := g.initParams(d)
	for i, p := range sig.Params {
		t := lowerType(p.BodyType())
		v := f.Param(t, paramConvention(p, t))
		if i < len(params) && params[i] != nil {
			g.locals[params[i]] = &local{value: v, typ: t}
		}
		if p.Name != "" {
			g.blk.DebugValue(v, p.Name, "let", "argno "+itoa(i+1))
		}
		g.destroyLater(v)
	}
	t := lowerType(recv)
	f.Param(sil.ThinMetatype(recv), sil.ParamUnowned)
	f.Type().Convention = sil.Method
	f.SetResult(t, resultConvention(t))

	g.initReturn = func() {
		if g.convSelf == nil {
			g.refuse(d, "a convenience initializer that returns before its self.init")
			g.blk.Unreachable()
			return
		}
		self := g.convSelf
		g.unwind()
		g.blk.Return(self)
	}
	g.block(d.Body)
	if g.blk != nil && g.blk.Term() == nil {
		g.initReturn()
	}
}

// classInitBody emits the entry point that fills an instance in: the
// declared parameters, then the instance itself as the receiver.
func (g *gen) classInitBody(d *ast.InitDecl, recv types.Type,
	sig *types.Signature, name string) {
	f := g.m.Func(name).SetSourceName("init").SetLinkage(g.initLinkage()).SetAttr("ossa")
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
		t := lowerType(p.BodyType())
		v := f.Param(t, paramConvention(p, t))
		if i < len(params) && params[i] != nil {
			g.locals[params[i]] = &local{value: v, typ: t}
		}
		if p.Name != "" {
			g.blk.DebugValue(v, p.Name, "let", "argno "+itoa(i+1))
		}
		g.destroyLater(v)
	}
	t := lowerType(recv)
	self := f.Param(t, sil.ParamGuaranteed)
	f.Type().Convention = sil.Method
	f.SetResult(t, resultConvention(t))
	g.blk.DebugValue(self, "self", "let")

	g.classFieldDefaults(recv, self)
	g.defaultAncestors(recv, self)

	// A subclass's initializer that does not run one of its superclass's
	// runs the superclass's init() at its end, as Swift has it.
	implicitSuper := g.implicitSuperInit(d, recv)
	g.initReturn = func() {
		if implicitSuper != nil {
			g.callSuperInit(d, recv, implicitSuper, nil)
		}
		g.unwind()
		g.blk.Return(g.blk.CopyValue(self))
	}

	g.block(d.Body)

	if g.blk != nil && g.blk.Term() == nil {
		g.initReturn()
	}
}

// implicitSuperInit is the superclass's init() a subclass's designated
// initializer runs at its end, where the body calls no super.init of its
// own; nil where there is none to run.
func (g *gen) implicitSuperInit(d *ast.InitDecl, recv types.Type) *types.Signature {
	cl, ok := recv.Underlying().(*types.Class)
	if !ok || cl.Superclass == nil || d == nil || d.Body == nil {
		return nil
	}
	calls := false
	ast.Inspect(d.Body, func(n ast.Node) bool {
		if ref, ok := n.(*ast.InitRefExpr); ok {
			if _, ok := ref.X.(*ast.SuperExpr); ok {
				calls = true
			}
		}
		return !calls
	})
	if calls {
		return nil
	}
	super, ok := cl.Superclass.Underlying().(*types.Class)
	if !ok {
		return nil
	}
	for _, sig := range super.Inits {
		if sig != nil && !sig.Convenience && len(sig.Params) == 0 {
			return sig
		}
	}
	return nil
}

// superInit is `super.init(...)` in a class's initializer: the
// superclass's initializing entry, run on this instance.
func (g *gen) superInit(e *ast.CallExpr, ref *ast.InitRefExpr) (*sil.Value, bool) {
	if _, ok := ref.X.(*ast.SuperExpr); !ok || g.recv == nil || !isClass(g.recv) {
		return nil, false
	}
	sig := g.info.Inits[e]
	if sig == nil {
		g.refuse(e, "a super.init whose arguments do not match one the superclass declares")
		return nil, true
	}
	var args []*ast.CallArg
	if e.Args != nil {
		args = e.Args.Args
	}
	if !g.callSuperInit(e, g.recv, sig, args) {
		return nil, true
	}
	return g.void(), true
}

// callSuperInit runs the superclass's initializer sig on self, the
// instance recv's initializer is filling in, with args for its parameters.
func (g *gen) callSuperInit(at ast.Node, recv types.Type, sig *types.Signature, args []*ast.CallArg) bool {
	cl, ok := recv.Underlying().(*types.Class)
	if !ok || cl.Superclass == nil {
		return false
	}
	super := cl.Superclass
	out := *sig
	out.Results = super
	alloc := g.initSymbol(super, &out)
	if alloc == "" || !strings.HasSuffix(alloc, "fC") {
		g.refuse(at, "this super.init")
		return false
	}
	st := lowerType(super)
	callee := g.m.Func(alloc[:len(alloc)-1] + "c").SetSourceName("init")
	if g.needsType(callee) {
		for _, p := range out.Params {
			pt := lowerType(p.Type)
			callee.Type().Params = append(callee.Type().Params,
				sil.Param{Type: pt, Convention: paramConvention(p, pt)})
		}
		callee.Type().Params = append(callee.Type().Params,
			sil.Param{Type: st, Convention: sil.ParamGuaranteed})
		callee.Type().Convention = sil.Method
		callee.SetResult(st, resultConvention(st))
	}
	var vals []*sil.Value
	if call, ok := at.(*ast.CallExpr); ok {
		var ok bool
		if vals, ok = g.arguments(call, &out); !ok {
			return false
		}
	}
	self := g.blk.Upcast(g.selfValue(), st)
	vals = append(vals, self)
	made := g.blk.Apply(g.blk.FunctionRef(callee), st, vals...)
	g.blk.DestroyValue(made)
	return true
}

// inheritedInitializers emits, for a class that inherits its superclass's
// designated initializers, the two entries of each: the one filling an
// instance in -- its own properties' defaults, then the superclass's
// initializer on it -- and the one making the instance.
func (g *gen) inheritedInitializers(recv types.Type) {
	cl, ok := recv.Underlying().(*types.Class)
	if !ok {
		return
	}
	for _, sig := range cl.Inits {
		if sig == nil || !sig.Inherited {
			continue
		}
		out := *sig
		out.Results = recv
		alloc := g.initSymbol(recv, &out)
		if alloc == "" || !strings.HasSuffix(alloc, "fC") {
			continue
		}
		initializing := alloc[:len(alloc)-1] + "c"
		g.inheritedInitBody(recv, &out, sig, initializing)
		g.classAllocator(recv, &out, alloc, initializing)
	}
}

// inheritedInitBody is the initializing entry of an inherited initializer.
func (g *gen) inheritedInitBody(recv types.Type, out, super *types.Signature, name string) {
	f := g.m.Func(name).SetSourceName("init").SetLinkage(g.initLinkage()).SetAttr("ossa")
	restore := g.saveFunction()
	defer restore()
	g.fn, g.entry = f, false
	f.Type().Params = nil
	g.locals = map[analyzer.Symbol]*local{}
	g.scopes, g.loops, g.pending = nil, nil, ""
	g.recv, g.self, g.initReturn = recv, nil, nil
	g.push()
	g.blk = f.Entry()

	var args []*sil.Value
	for _, p := range out.Params {
		pt := lowerType(p.Type)
		args = append(args, f.Param(pt, paramConvention(p, pt)))
	}
	t := lowerType(recv)
	self := f.Param(t, sil.ParamGuaranteed)
	f.Type().Convention = sil.Method
	f.SetResult(t, resultConvention(t))
	g.classFieldDefaults(recv, self)
	g.defaultAncestors(recv, self)

	cl := recv.Underlying().(*types.Class)
	superT := cl.Superclass
	sout := *super
	sout.Results = superT
	alloc := g.initSymbol(superT, &sout)
	st := lowerType(superT)
	callee := g.m.Func(alloc[:len(alloc)-1] + "c").SetSourceName("init")
	if g.needsType(callee) {
		for _, p := range sout.Params {
			pt := lowerType(p.Type)
			callee.Type().Params = append(callee.Type().Params,
				sil.Param{Type: pt, Convention: paramConvention(p, pt)})
		}
		callee.Type().Params = append(callee.Type().Params,
			sil.Param{Type: st, Convention: sil.ParamGuaranteed})
		callee.Type().Convention = sil.Method
		callee.SetResult(st, resultConvention(st))
	}
	made := g.blk.Apply(g.blk.FunctionRef(callee), st, append(args, g.blk.Upcast(self, st))...)
	g.blk.DestroyValue(made)
	g.unwind()
	g.blk.Return(g.blk.CopyValue(self))
}

// classAllocator emits the entry point that makes the instance and
// hands it to the one that fills it in.
func (g *gen) classAllocator(recv types.Type, sig *types.Signature, name, body string) {
	f := g.m.Func(name).SetSourceName("init").SetLinkage(g.initLinkage()).SetAttr("ossa")
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
	var args []*sil.Value
	for _, p := range sig.Params {
		pt := lowerType(p.Type)
		args = append(args, f.Param(pt, paramConvention(p, pt)))
	}
	f.Param(sil.ThinMetatype(recv), sil.ParamUnowned)
	f.Type().Convention = sil.Method
	f.SetResult(t, resultConvention(t))

	callee := g.m.Func(body).SetSourceName("init")
	if g.needsType(callee) {
		for _, p := range sig.Params {
			pt := lowerType(p.Type)
			callee.Type().Params = append(callee.Type().Params,
				sil.Param{Type: pt, Convention: paramConvention(p, pt)})
		}
		callee.Type().Params = append(callee.Type().Params,
			sil.Param{Type: t, Convention: sil.ParamGuaranteed})
		callee.Type().Convention = sil.Method
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
		if cand != nil && g.sameInitParams(cand, d) {
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
	var inits []*types.Signature
	if st, ok := recv.Underlying().(*types.Struct); ok {
		inits = st.Inits
	} else if en, ok := recv.Underlying().(*types.Enum); ok {
		inits = en.Inits
	} else if b := g.info.Builtins[analyzer.BuiltinKey(recv)]; b != nil && isBasicValue(recv) {
		// Int's and String's are the ones their extensions declare.
		inits = b.Inits
	}
	for _, cand := range inits {
		if cand != nil && g.sameInitParams(cand, d) {
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
func (g *gen) sameInitParams(sig *types.Signature, d *ast.InitDecl) bool {
	var params []*ast.Param
	if d.Sig != nil {
		params = d.Sig.Params
	}
	if len(sig.Params) != len(params) {
		return false
	}
	// By label and name too: init(nanos:) and init(label:) take one
	// parameter each, and counting them made both bodies one function.
	// Read in the file the initializer was written in, which for a type
	// another module declares is not the file being lowered.
	file := g.fileOf(d)
	if file == nil {
		file = g.file
	}
	for i, p := range params {
		label, name := "", ""
		if p.Label != nil {
			label = p.Label.Text(file)
		}
		if p.Name != nil {
			name = p.Name.Text(file)
		} else if label != "_" {
			name = label
		}
		if sig.Params[i].Label != label || sig.Params[i].Name != name {
			return false
		}
	}
	// And by type: init?(exactly: Int) and init?(exactly: Double) have
	// the same labels and names. A type written in a generic parameter is
	// substituted in an instance's signature, and is left to the labels.
	prev := g.file
	g.file = file
	syms := g.initParams(d)
	g.file = prev
	for i, sym := range syms {
		if sym == nil || i >= len(sig.Params) {
			continue
		}
		// A variadic parameter binds the array of what it takes.
		declared, called := sym.Type(), sig.Params[i].BodyType()
		if declared == nil || called == nil || mentionsTypeParam(declared) || mentionsTypeParam(called) {
			continue
		}
		if !types.Identical(declared, called) {
			return false
		}
	}
	return true
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
func (g *gen) classMetatype(t types.Type) (*sil.Value, bool) {
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

// emitCoreInit emits the body of an initializer the core's source gives
// Int or String -- `Int32(clamping:)` -- the first time a module calls it.
// Like the core's functions it is lowered where it is used, privately,
// so that each module has its own copy and none clashes with another's.
func (g *gen) emitCoreInit(recv types.Type, sig *types.Signature) {
	core := g.info.CoreAlgorithms
	if core == nil {
		return
	}
	name := g.initSymbol(recv, sig)
	if name == "" {
		return
	}
	if f := g.m.Lookup(name); f != nil && !f.IsDeclaration() {
		return
	}
	for _, st := range core.Stmts {
		decl, ok := st.(*ast.DeclStmt)
		if !ok {
			continue
		}
		ext, ok := decl.D.(*ast.ExtensionDecl)
		if !ok || ext.Body == nil || !types.Identical(g.info.Extensions[ext], recv) {
			continue
		}
		for _, mem := range ext.Body.Members {
			d, ok := mem.(*ast.InitDecl)
			if !ok || !g.sameInitParams(sig, d) {
				continue
			}
			restore := g.apart()
			g.file, g.specializing = core.Unit, true
			g.initializer(d, recv)
			restore()
			return
		}
	}
}

// isBasicValue reports whether t is one of the core's value types that
// the universe declares rather than a struct: Int, Double, Bool, String.
// Each lowers as a struct of one field, and an initializer an extension
// gives it is a struct's initializer.
func isBasicValue(t types.Type) bool {
	if t == nil {
		return false
	}
	b, ok := t.Underlying().(*types.Basic)
	return ok && b.Info()&types.IsUntyped == 0 && b.Kind() != types.Invalid
}

func isEnumType(t types.Type) bool {
	if t == nil {
		return false
	}
	_, ok := t.Underlying().(*types.Enum)
	return ok
}

func isStructType(t types.Type) bool {
	if t == nil {
		return false
	}
	_, ok := t.Underlying().(*types.Struct)
	return ok
}

// callClassInit calls a class's allocating initializer.
func (g *gen) callClassInit(e *ast.CallExpr, t types.Type, cl *types.Class) *sil.Value {
	var args []*ast.CallArg
	if e.Args != nil {
		args = e.Args.Args
	}
	sig := g.info.Inits[e]
	if sig == nil {
		sig = pickInitFrom(cl.Inits, args)
	}
	if sig == nil {
		g.refuse(e, "a call to an initializer whose arguments do not match one the class declares")
		return nil
	}
	if v, generic := g.genericClassInit(e, t, sig, args); generic {
		return v
	}
	out := *sig
	out.Results = t
	return g.applyInit(e, t, &out, args)
}

func (g *gen) callInit(e *ast.CallExpr, t types.Type, st *types.Struct) *sil.Value {
	var args []*ast.CallArg
	if e.Args != nil {
		args = e.Args.Args
	}
	sig := g.info.Inits[e]
	if sig == nil {
		sig = pickInit(st, args)
	}
	if sig == nil {
		g.refuse(e, "a call to an initializer whose arguments do not match one it declares")
		return nil
	}
	if v, generic := g.genericStructInit(e, t, sig, args); generic {
		return v
	}
	// A Task is the handle the runtime makes for the task it starts.
	if symbol, ok := g.coreTask(t, "init"); ok {
		return g.startTask(e, t, symbol, sig)
	}
	out := *sig
	out.Results = t
	return g.applyInit(e, t, &out, args)
}

// delegatingInit is `self.init(...)` in a struct's initializer: the
// other initializer makes the value, and self is it, as `self = T(...)`
// would have it.
func (g *gen) delegatingInit(e *ast.CallExpr, ref *ast.InitRefExpr) (*sil.Value, bool) {
	if _, isSelf := ref.X.(*ast.SelfExpr); isSelf && g.convenience && g.recv != nil {
		return g.convenienceDelegation(e)
	}
	if _, isSelf := ref.X.(*ast.SelfExpr); !isSelf || g.self == nil || g.recv == nil || isClass(g.recv) {
		return nil, false
	}
	st, ok := g.recv.Underlying().(*types.Struct)
	if !ok {
		return nil, false
	}
	var v *sil.Value
	if g.info.Inits[e] == nil && st.Memberwise() != nil {
		// An extension's initializer handing on to the memberwise one.
		v = g.memberwise(e, g.recv, st)
	} else {
		v = g.callInit(e, g.recv, st)
	}
	if v == nil {
		return nil, true
	}
	v = g.consume(v)
	access := g.blk.BeginAccess(g.self.addr, "modify", "unknown")
	g.blk.Assign(v, access)
	g.blk.EndAccess(access)
	return g.void(), true
}

// metatypeInit is `T.init(...)`: through a class's name, that class's
// initializer; through a metatype value -- `type.init(id:)` with type a
// Widget.Type -- the required initializer of whichever class the value
// is, found in its table.
func (g *gen) metatypeInit(e *ast.CallExpr, ref *ast.InitRefExpr) (*sil.Value, bool) {
	meta, ok := g.typeOf(ref.X).(*types.Metatype)
	if !ok {
		return nil, false
	}
	t := g.substituted(meta.Instance)
	cl, ok := receiverClass(t)
	if !ok {
		if st, isStruct := t.Underlying().(*types.Struct); isStruct {
			if st.Memberwise() != nil && g.info.Inits[e] == nil {
				return g.memberwise(e, t, st), true
			}
			return g.callInit(e, t, st), true
		}
		return nil, false
	}
	if id, named := ref.X.(*ast.IdentExpr); named && id.Name != nil {
		if _, isType := g.info.Uses[id.Name].(*analyzer.TypeNameSymbol); isType {
			return g.callClassInit(e, t, cl), true
		}
	}
	sig := g.info.Inits[e]
	if sig == nil || !sig.Required {
		g.refuse(e, "an initializer called through a metatype that is not required")
		return nil, true
	}
	self := g.expr(ref.X)
	if self == nil {
		return nil, true
	}
	out := *sig
	out.Results = t
	ct := lowerType(t)
	ft := &sil.FuncType{Convention: sil.Method}
	for _, p := range out.Params {
		pt := lowerType(p.Type)
		ft.Params = append(ft.Params, sil.Param{Type: pt, Convention: paramConvention(p, pt)})
	}
	ft.Params = append(ft.Params, sil.Param{Type: sil.ThinMetatype(t), Convention: sil.ParamUnowned})
	ft.Results = append(ft.Results, sil.Result{Type: ct, Convention: resultConvention(ct)})
	intro := initIntroducer(cl, sig)
	method := g.blk.ClassMethod(self, intro.Name+"."+initSlotKey(sig), sil.Object(ft))
	vals, ok := g.arguments(e, &out)
	if !ok {
		return nil, true
	}
	vals = append(vals, g.blk.Metatype(ct))
	v := g.blk.Apply(method, ct, vals...)
	g.destroyLater(v)
	return v, true
}

// convenienceDelegation is `self.init(...)` in a convenience initializer:
// the other initializer makes the instance, which is self from here on.
func (g *gen) convenienceDelegation(e *ast.CallExpr) (*sil.Value, bool) {
	cl, ok := g.recv.Underlying().(*types.Class)
	if !ok {
		return nil, false
	}
	if g.convSelf != nil {
		g.refuse(e, "a second self.init in one initializer")
		return nil, true
	}
	// Where it runs on every path: in the body's own statements, before
	// anything branches.
	if g.blk != g.fn.Entry() {
		g.refuse(e, "a self.init anywhere but in the initializer's own statements")
		return nil, true
	}
	v := g.callClassInit(e, g.recv, cl)
	if v == nil {
		return nil, true
	}
	g.convSelf = g.consume(v)
	return g.void(), true
}

// importedType reports whether another compiled module declared t, and so
// emitted whatever this module would otherwise have to.
//
// A core type is not one of those. It is named as the standard library's
// so that every module spells it the same way, but no library ships it --
// this compiler's runtime provides the type, and each module that needs
// its metadata emits a copy of its own, internal to that module.
func (g *gen) importedType(t types.Type) bool {
	if g.isCoreType(t) {
		return false
	}
	m := g.moduleOfType(t)
	return m != "" && m != g.module
}

// applyInit emits the call to an initializer this side does not
// define.
func (g *gen) applyInit(e *ast.CallExpr, t types.Type, out *types.Signature, args []*ast.CallArg) *sil.Value {
	name := g.initSymbol(t, out)
	if name == "" {
		g.refuse(e, "this initializer")
		return nil
	}
	return g.applyInitNamed(e, t, out, args, name)
}

// applyInitNamed emits the call to the initializer with this symbol.
func (g *gen) applyInitNamed(e *ast.CallExpr, t types.Type, out *types.Signature, args []*ast.CallArg, name string) *sil.Value {
	made := t
	if out.Failable {
		made = &types.Optional{Wrapped: t}
	}
	ref := g.initRef(t, out, name)

	// Evaluate call arguments respecting parameter conventions.
	vals, ok := g.arguments(e, out)
	if !ok {
		return nil
	}
	// An imported class's initializer is its module's, which takes the
	// class's metadata; an instance of an imported generic class is made
	// by a specialization lowered here, which takes none.
	_, specialized := t.(*types.GenericInstance)
	if isClass(t) && g.importedType(t) && !specialized {
		meta, ok := g.classMetatype(t)
		if !ok {
			g.refuse(e, "an initializer of a class whose metadata cannot be named")
			return nil
		}
		vals = append(vals, meta)
	} else {
		vals = append(vals, g.blk.Metatype(lowerType(t)))
	}

	// A throwing initializer is called like any throwing function: through
	// a try_apply whose error edge raises to the enclosing catch or out of
	// the caller. `try?`/`try!` written on the call are honored.
	if out.Throws {
		optional := false
		if pendingOptional, pendingTrap := g.tryOn(e); pendingOptional || pendingTrap {
			optional = pendingOptional
			g.tryBang = pendingTrap
		}
		return g.tryApply(e, ref, vals, made, optional, false)
	}

	v := g.blk.Apply(ref, lowerType(made), vals...)
	g.destroyLater(v)
	return v
}

// initRef is a reference to the initializer of t with this symbol, typed
// for a call: the parameters out declares, then the metatype.
func (g *gen) initRef(t types.Type, out *types.Signature, name string) *sil.Value {
	made := t
	if out.Failable {
		made = &types.Optional{Wrapped: t}
	}
	callee := g.m.Func(name).SetSourceName("init")
	if g.needsType(callee) {
		for _, p := range out.Params {
			pt := lowerType(p.Type)
			callee.Type().Params = append(callee.Type().Params,
				sil.Param{Type: pt, Convention: paramConvention(p, pt)})
		}
		self := sil.ThinMetatype(t)
		if isClass(t) && g.importedType(t) {
			self = sil.ThickMetatype(t)
		}
		callee.Type().Params = append(callee.Type().Params,
			sil.Param{Type: self, Convention: sil.ParamUnowned})
		callee.Type().Convention = sil.Method
		callee.SetResult(lowerType(made), resultConvention(lowerType(made)))
		if out.Throws {
			callee.SetThrows(sil.Object(sil.BuiltinNativeObj))
		}
	}
	return g.blk.FunctionRef(callee)
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

// deinitializer emits a class's deinit method.
func (g *gen) deinitializer(d *ast.DeinitDecl, recv types.Type) {
	if d.Body == nil || !isClass(recv) {
		return
	}
	f := g.m.Func(deinitSymbol(g.module, recv)).SetSourceName("deinit").SetAttr("ossa")
	restore := g.saveFunction()
	defer restore()

	g.fn, g.entry = f, false
	f.Type().Params = nil
	g.locals = map[analyzer.Symbol]*local{}
	g.scopes, g.loops, g.pending = nil, nil, ""
	g.recv, g.self, g.initReturn = recv, nil, nil
	g.push()
	g.blk = f.Entry()

	t := lowerType(recv)
	self := f.Param(t, sil.ParamGuaranteed)
	f.Type().Convention = sil.Method
	g.blk.DebugValue(self, "self", "let")

	g.block(d.Body)
	if g.blk != nil && g.blk.Term() == nil {
		g.unwind()
		g.blk.Return(g.void())
	}
}

// structDeinitializer emits a ~Copyable struct's deinit: a function that
// takes the value, runs the body with it as self, and then lets go of
// what the value holds. Where a value of the struct ends -- the end of
// its scope, a consuming method's end -- this is what is called.
func (g *gen) structDeinitializer(d *ast.DeinitDecl, recv types.Type) {
	if d.Body == nil {
		return
	}
	f := g.m.Func(deinitSymbol(g.module, recv)).SetSourceName("deinit").SetAttr("ossa")
	if !f.IsDeclaration() && len(f.Blocks()) > 0 && f.Entry().Term() != nil {
		return
	}
	restore := g.saveFunction()
	defer restore()

	g.fn, g.entry = f, false
	f.Type().Params = nil
	g.locals = map[analyzer.Symbol]*local{}
	g.scopes, g.loops, g.pending = nil, nil, ""
	g.recv, g.self, g.initReturn = recv, nil, nil
	g.push()
	g.blk = f.Entry()

	t := lowerType(recv)
	self := f.Param(t, sil.ParamOwned)
	f.Type().Convention = sil.Method
	g.blk.DebugValue(self, "self", "let")

	g.block(d.Body)
	if g.blk != nil && g.blk.Term() == nil {
		g.unwind()
		// What the value holds is let go of as a value's is; its deinit
		// has run.
		g.blk.DestroyValue(self)
		g.blk.Return(g.void())
	}
}

// structDeinit is the deinit a value of t ends with, or nil.
func (g *gen) structDeinit(t types.Type) *sil.Func {
	st, ok := t.Underlying().(*types.Struct)
	if !ok || !st.Deinit {
		return nil
	}
	f := g.m.Func(deinitSymbol(g.module, t)).SetSourceName("deinit")
	if g.needsType(f) {
		f.Type().Params = []sil.Param{{Type: lowerType(t), Convention: sil.ParamOwned}}
		f.Type().Convention = sil.Method
	}
	return f
}

// endValue ends an owned value: its deinit, for a ~Copyable struct that
// has one, and otherwise destroy_value.
func (g *gen) endValue(v *sil.Value) {
	if v != nil && v.Type().Formal() != nil {
		if f := g.structDeinit(v.Type().Formal()); f != nil {
			g.blk.Apply(g.blk.FunctionRef(f), sil.Object(types.Typ[types.Void]), v)
			return
		}
	}
	g.blk.DestroyValue(v)
}

// takeLocal is `consume x`: a let's value, which is the caller's from
// here on; the let's own end is forgotten. Nil where x is not a local
// held as a value.
func (g *gen) takeLocal(x ast.Expr) *sil.Value {
	id, ok := unparen(x).(*ast.IdentExpr)
	if !ok || id.Name == nil {
		return nil
	}
	l := g.locals[g.info.Uses[id.Name]]
	if l == nil || l.value == nil || l.value.Ownership() != sil.Owned {
		return nil
	}
	g.forget(l.value)
	return l.value
}

// deinitSymbol names a class's deinit. It is called only from the class's
// destroyer in the same module, so the name is this compiler's.
func deinitSymbol(module string, recv types.Type) string {
	return "$sVSCdeinit_" + identifierSafe(module) + "_" + identifierSafe(typeNameOf(recv))
}

// initLinkage is an initializer's: public, as it has always been, except
// for a specialization, which is this module's own copy.
func (g *gen) initLinkage() sil.Linkage {
	if g.specializing {
		return sil.Private
	}
	if g.inlinable && !g.specializing {
		return sil.PublicExternal
	}
	return sil.Public
}

// classFieldDefaults gives a class's own stored properties with a default
// value that value, in the instance self, before an initializer's body
// runs, as a struct's have theirs. The instance starts zeroed, so there
// is nothing for the store to let go of.
func (g *gen) classFieldDefaults(recv types.Type, self *sil.Value) {
	if cl, ok := recv.Underlying().(*types.Class); ok {
		for _, fld := range cl.Fields {
			if fld == nil || !fld.HasDefault {
				continue
			}
			def := g.info.FieldDefaults[fld]
			if def == nil {
				// An instance of a generic class: the default is recorded
				// against the declaration's property, and the
				// specialization in force reads it for this instance.
				def, _ = g.instanceDefault(recv, fld.Name)
			}
			if def == nil {
				continue
			}
			v := g.rvalue(def)
			if v == nil {
				continue
			}
			ft := lowerType(fld.Type)
			v = g.optionalFor(def, v, g.typeOf(def), fld.Type)
			at := g.blk.RefElementAddr(self, memberName(recv, fld.Name), ft)
			g.blk.Store(v, at, storeQualifier(ft))
		}
	}
}

// defaultAncestors gives the properties of the superclasses above recv
// that declare no initializer their defaults, in self: such a class is
// made by its implicit init(), which is those defaults and nothing else,
// and which has no entry of its own to run.
func (g *gen) defaultAncestors(recv types.Type, self *sil.Value) {
	cl, ok := recv.Underlying().(*types.Class)
	if !ok {
		return
	}
	for up := cl.Superclass; up != nil; {
		ucl, ok := up.Underlying().(*types.Class)
		if !ok || len(ucl.Inits) > 0 {
			return
		}
		g.classFieldDefaults(up, self)
		up = ucl.Superclass
	}
}
