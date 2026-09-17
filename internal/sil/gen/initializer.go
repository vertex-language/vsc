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
	g.structInitBody(d, recv, sig, name)
}

// structInitBody emits a struct initializer's body under this symbol:
// the declaration's own, or one specialized for a generic struct's
// instance.
func (g *gen) structInitBody(d *ast.InitDecl, recv types.Type, sig *types.Signature, name string) {
	f := g.m.Func(name).SetSourceName("init").SetAttr("ossa")

	outer := struct {
		fn      *sil.Func
		entry   bool
		blk     *sil.Block
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
	mt := sil.ThinMetatype(recv)
	f.Param(mt, sil.ParamUnowned)
	f.Type().Convention = sil.Method
	f.SetResult(lowerType(recv), resultConvention(lowerType(recv)))

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

// classInitializer lowers a class initializer into allocating (fC) and initializing (fc) functions.
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
	t := lowerType(recv)
	self := f.Param(t, sil.ParamGuaranteed)
	f.Type().Convention = sil.Method
	f.SetResult(t, resultConvention(t))
	g.blk.DebugValue(self, "self", "let")

	// The class's own stored properties with a default value have it
	// before the body runs, as a struct's do. The instance starts zeroed,
	// so there is nothing for the store to let go of.
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
	st, ok := recv.Underlying().(*types.Struct)
	if !ok {
		return nil
	}
	for _, cand := range st.Inits {
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
	for i, p := range params {
		label, name := "", ""
		if p.Label != nil {
			label = g.text(p.Label)
		}
		if p.Name != nil {
			name = g.text(p.Name)
		} else if label != "_" {
			name = label
		}
		if sig.Params[i].Label != label || sig.Params[i].Name != name {
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

func isStructType(t types.Type) bool {
	if t == nil {
		return false
	}
	_, ok := t.Underlying().(*types.Struct)
	return ok
}

// callClassInit calls a class's allocating initializer.
func (g *gen) callClassInit(e *ast.CallExpr, t types.Type, cl *types.Class) *sil.Value {
	if cl.Superclass != nil && !g.importedType(t) {
		g.refuse(e, "an initializer of a class with a superclass, which has to run "+
			"the one above it")
		return nil
	}
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
		callee.SetResult(lowerType(t), resultConvention(lowerType(t)))
	}
	ref := g.blk.FunctionRef(callee)

	// Evaluate call arguments respecting parameter conventions.
	vals, ok := g.arguments(e, out)
	if !ok {
		return nil
	}
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

// deinitSymbol names a class's deinit. It is called only from the class's
// destroyer in the same module, so the name is this compiler's.
func deinitSymbol(module string, recv types.Type) string {
	return "$sVSCdeinit_" + identifierSafe(module) + "_" + identifierSafe(typeNameOf(recv))
}
