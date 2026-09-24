package gen

import (
	"github.com/vertex-language/vsc/analyzer"
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/core"
	"github.com/vertex-language/vsc/internal/sil"
	"github.com/vertex-language/vsc/stdlib"
	"github.com/vertex-language/vsc/types"
)

// asyncEntryName is the symbol an async main's body is given, the name
// main being the entry that runs it.
const asyncEntryName = "$vsc_async_main"

// asyncEntry emits the program's entry for an async main: it runs body as
// the first task, returns when that task has finished, and exits with 0.
func (g *gen) asyncEntry(name string, body *sil.Func) {
	f := g.m.Func(name).SetSourceName(EntryName).SetLinkage(sil.Public).SetAttr("ossa")
	entrySignature(f)

	prevFn, prevBlk, prevScopes := g.fn, g.blk, g.scopes
	defer func() { g.fn, g.blk, g.scopes = prevFn, prevBlk, prevScopes }()
	g.fn, g.blk, g.scopes = f, f.Entry(), nil
	g.push()

	// A main returning Int32 hands its status back through the executor.
	if results := body.Type().Results; len(results) == 1 {
		sig := &types.Signature{Results: entryResult(), Async: true}
		fn := g.blk.ThinToThickFunction(g.blk.FunctionRef(body), lowerType(sig))
		g.destroyLater(fn)
		status := g.runtimeResult(stdlib.AsyncMainStatus,
			[]sil.Param{{Type: lowerType(sig), Convention: sil.ParamGuaranteed}},
			lowerType(entryResult()), fn)
		g.unwind()
		g.blk.Return(status)
		return
	}
	sig := &types.Signature{Results: types.Typ[types.Void], Async: true}
	fn := g.blk.ThinToThickFunction(g.blk.FunctionRef(body), lowerType(sig))
	g.destroyLater(fn)
	g.runtimeResult(stdlib.AsyncMain,
		[]sil.Param{{Type: lowerType(sig), Convention: sil.ParamGuaranteed}},
		lowerType(types.Typ[types.Void]), fn)
	g.unwind()
	g.blk.Return(g.entryStatus())
}

// coreTask is the runtime function a member of core's Task is, for a type
// that is core's Task and not one a program declared of the same name.
func (g *gen) coreTask(t types.Type, member string) (string, bool) {
	t = taskBase(t)
	if t == nil || !g.info.CoreTypes[t] {
		return "", false
	}
	if st, ok := t.Underlying().(*types.Struct); !ok || st.Name != "Task" {
		return "", false
	}
	return core.LowerTask(member)
}

// taskCall calls the runtime for a member of Task. None of them fails --
// sleep is declared throws for cancellation, which there is none of -- so
// a try written on one has nothing to catch, and the call is a plain one.
// result is what the call is, or nil for nothing.
func (g *gen) taskCall(e *ast.CallExpr, symbol string, sig *types.Signature, result types.Type) *sil.Value {
	g.tryOn(e)
	var params []sil.Param
	var args []*sil.Value
	if e.Args != nil {
		for i, a := range e.Args.Args {
			if i >= len(sig.Params) {
				g.refuse(e, "a call to Task with more arguments than it takes")
				return nil
			}
			v := g.rvalue(a.X)
			if v == nil {
				return nil
			}
			pt := lowerType(sig.Params[i].Type)
			conv := sil.ParamUnowned
			if !pt.Trivial() {
				// Borrowed by the runtime, so the value is still this
				// scope's to let go of: a task retains the context of the
				// closure it runs, and releases it when it has finished.
				conv = sil.ParamGuaranteed
				g.destroyLater(v)
			}
			params = append(params, sil.Param{Type: pt, Convention: conv})
			args = append(args, v)
		}
	}
	if result != nil {
		return g.runtimeResult(symbol, params, lowerType(result), args...)
	}
	g.runtimeResult(symbol, params, lowerType(types.Typ[types.Void]), args...)
	return g.void()
}

// taskValue lowers `task.value`: waiting until the task has finished,
// which is all the value core's Task has.
func (g *gen) taskValue(e *ast.MemberExpr) (*sil.Value, bool) {
	if e.Name == nil || g.text(e.Name) != "value" {
		return nil, false
	}
	t := g.typeOf(e.X)
	symbol, ok := g.coreTask(t, "value")
	if !ok {
		return nil, false
	}
	named := taskBase(t)
	_, field, found := storedField(named, "handle")
	if !found {
		g.refuse(e, "core's Task without its handle")
		return nil, true
	}
	v := g.expr(e.X)
	if v == nil {
		return nil, true
	}
	ht := lowerType(field.Type)
	base := v
	if v.Ownership() == sil.Owned {
		base = g.blk.BeginBorrow(v)
	}
	handle := g.blk.StructExtract(base, memberName(named, "handle"), ht)
	g.runtimeResult(symbol, []sil.Param{{Type: ht, Convention: sil.ParamGuaranteed}},
		lowerType(types.Typ[types.Void]), handle)
	out := g.void()
	// What the operation returned is in the cell, now that it has.
	if result := taskResultOf(t); result != nil {
		cell := g.blk.StructExtract(base, memberName(named, "result"), ht)
		contents := g.taskCellContents(cell, ht, result)
		rt := lowerType(result)
		out = g.loaded(g.blk.Load(g.blk.PointerToAddress(contents, rt.Address()), loadQualifier(rt)), rt)
	}
	if base != v {
		g.blk.EndBorrow(base)
	}
	return out, true
}

// taskBase is core's Task for a Task of some result.
func taskBase(t types.Type) types.Type {
	if gi, ok := t.(*types.GenericInstance); ok {
		return gi.Base
	}
	return t
}

// taskResultOf is what a Task's operation returns, or nil for nothing.
func taskResultOf(t types.Type) types.Type {
	if gi, ok := t.(*types.GenericInstance); ok && len(gi.Args) > 0 {
		return gi.Args[0]
	}
	return nil
}

// startTask lowers `Task { ... }`: a cell for what the operation returns,
// the task, and the Task value holding both. An operation that returns
// something runs through a thunk that stores it in the cell.
func (g *gen) startTask(e *ast.CallExpr, t types.Type, symbol string, sig *types.Signature) *sil.Value {
	named := taskBase(t)
	_, field, found := storedField(named, "handle")
	if !found || e.Args == nil || len(e.Args.Args) != 1 {
		g.refuse(e, "core's Task without its handle")
		return nil
	}
	cellT := lowerType(field.Type)
	result := taskResultOf(t)
	size := int64(0)
	var opSig *types.Signature
	if result != nil {
		opSig, _ = g.typeOf(e.Args.Args[0].X).Underlying().(*types.Signature)
		if opSig == nil || opSig.Throws {
			g.refuse(e, "a task whose operation throws")
			return nil
		}
		size = types.Sizeof(result, types.DefaultTarget64)
	}
	var cell *sil.Value
	if result != nil && !lowerType(result).Trivial() {
		// A value that owns something is let go with the cell, through
		// its type's witnesses.
		meta, ok := g.stdlibMetadata(e, result)
		if !ok {
			return nil
		}
		cell = g.runtimeResult(stdlib.TaskCellTyped,
			[]sil.Param{{Type: rawPointerType(), Convention: sil.ParamUnowned}}, cellT, meta)
	} else {
		word := sil.Object(sil.BuiltinInt64)
		cell = g.runtimeResult(stdlib.TaskCell,
			[]sil.Param{{Type: word, Convention: sil.ParamUnowned}},
			cellT, g.blk.IntegerLiteral(word, size))
	}
	var handle *sil.Value
	if result == nil {
		handle = g.taskCall(e, symbol, sig, field.Type)
	} else {
		g.tryOn(e)
		op := g.rvalue(e.Args.Args[0].X)
		if op == nil {
			return nil
		}
		voidSig := lowerType(&types.Signature{Results: types.Typ[types.Void], Async: true})
		thunk := g.taskResultThunk(opSig, result, cellT)
		operation := g.blk.PartialApply(g.blk.FunctionRef(thunk), voidSig,
			g.consume(op), g.blk.CopyValue(cell))
		g.destroyLater(operation)
		handle = g.runtimeResult(symbol,
			[]sil.Param{{Type: voidSig, Convention: sil.ParamGuaranteed}}, cellT, operation)
	}
	if handle == nil {
		return nil
	}
	return g.blk.Struct(lowerType(t), handle, cell)
}

// taskResultThunk is the operation a task returning a value runs: the
// program's operation, then what it returned stored in the cell its Task
// reads it from.
func (g *gen) taskResultThunk(opSig *types.Signature, result types.Type, cellT sil.Type) *sil.Func {
	f := g.m.Func(g.closureSymbol()).SetLinkage(sil.Private).SetAttr("ossa")
	outer := struct {
		fn      *sil.Func
		entry   bool
		blk     *sil.Block
		scopes  []*scope
		locals  map[analyzer.Symbol]*local
		loops   []loop
		pending string
		recv    types.Type
	}{g.fn, g.entry, g.blk, g.scopes, g.locals, g.loops, g.pending, g.recv}
	defer func() {
		g.fn, g.entry, g.blk = outer.fn, outer.entry, outer.blk
		g.scopes, g.locals = outer.scopes, outer.locals
		g.loops, g.pending, g.recv = outer.loops, outer.pending, outer.recv
	}()
	g.fn, g.entry = f, false
	g.locals = map[analyzer.Symbol]*local{}
	g.scopes, g.loops, g.pending, g.recv = nil, nil, "", nil
	g.push()
	g.blk = f.Entry()

	// It runs the operation and waits for it, so it is async itself and
	// its type has to say so: it is what the task is entered at, and a
	// task is entered the way an async function is called.
	f.Type().Async = true

	op := f.Param(lowerType(opSig), sil.ParamGuaranteed)
	cell := f.Param(cellT, sil.ParamGuaranteed)
	rt := lowerType(result)
	v := g.blk.Apply(op, rt)
	contents := g.taskCellContents(cell, cellT, result)
	g.blk.Store(v, g.blk.PointerToAddress(contents, rt.Address()), storeQualifier(rt))
	g.blk.Return(g.void())
	return f
}

// taskCellContents is where a Task's cell keeps what its operation
// returned: past a plain cell's header, or where a typed cell's value is.
func (g *gen) taskCellContents(cell *sil.Value, cellT sil.Type, result types.Type) *sil.Value {
	symbol := stdlib.TaskCellData
	if !lowerType(result).Trivial() {
		symbol = stdlib.TaskTypedData
	}
	return g.runtimeResult(symbol,
		[]sil.Param{{Type: cellT, Convention: sil.ParamGuaranteed}}, rawPointerType(), cell)
}

// coreMainActor is the runtime function a member of core's MainActor
// is, for a type that is core's MainActor and not a program's own.
func (g *gen) coreMainActor(t types.Type, member string) (string, bool) {
	if t == nil || !g.info.CoreTypes[t] {
		return "", false
	}
	if en, ok := t.Underlying().(*types.Enum); !ok || en.Name != "MainActor" {
		return "", false
	}
	return core.LowerMainActor(member)
}

// mainActorCall lowers a static member of MainActor. `hop(n)` is the
// runtime's, as it is. `run { }` is its closure called: the closure is
// @MainActor and async, so its own prologue gets the task to the main
// thread, and the await it is written under brings the task back (see
// await). `assumeIsolated { }` is the body called here, after the
// runtime has checked that here is the main thread.
func (g *gen) mainActorCall(e *ast.CallExpr, symbol string, sig *types.Signature) *sil.Value {
	if e.Args == nil || len(e.Args.Args) != 1 {
		g.refuse(e, "a member of MainActor without its argument")
		return nil
	}
	body := e.Args.Args[0].X
	if _, isClosure := g.typeOf(body).Underlying().(*types.Signature); !isClosure {
		return g.taskCall(e, symbol, sig, nil)
	}
	if symbol == stdlib.TaskAssumeMain {
		g.runtimeResult(symbol, nil, lowerType(types.Typ[types.Void]))
	}
	if v := g.applyValue(&ast.CallExpr{Span: e.Span, Fun: body}, body); v == nil {
		return nil
	}
	return g.void()
}

// Where a hop goes, as the runtime numbers them.
const (
	hopMain = 0 // the main executor, Thread 0
	hopPool = 1 // a worker of the pool
	hopHome = 2 // the executor the task calls home
)

// isolated is whether the function being lowered is @MainActor.
func (g *gen) isolated() bool {
	return g.fn != nil && g.fn.Type().Isolated
}

// hop moves the task to another executor: a suspension the runtime
// resumes on the thread it names, once the task's frames have unwound.
func (g *gen) hop(where int64) {
	word := sil.Object(sil.BuiltinInt64)
	g.runtimeResult(stdlib.TaskHop,
		[]sil.Param{{Type: word, Convention: sil.ParamUnowned}},
		lowerType(types.Typ[types.Void]), g.blk.IntegerLiteral(word, where))
}

// ensure hops to an executor unless the task is there already, which
// the runtime says without suspending. Nothing outside an async
// function, which cannot suspend and so is where its caller was.
func (g *gen) ensure(where int64) {
	if g.fn == nil || !g.fn.Type().Async || g.blk == nil {
		return
	}
	word := sil.Object(sil.BuiltinInt64)
	needs := g.runtimeResult(stdlib.TaskNeedsHop,
		[]sil.Param{{Type: word, Convention: sil.ParamUnowned}}, word,
		g.blk.IntegerLiteral(word, where))
	bit := g.blk.Builtin("cmp_ne_Int64", sil.Object(sil.BuiltinInt1), needs,
		g.blk.IntegerLiteral(word, 0))
	move, join := g.fn.Block(), g.fn.Block()
	g.blk.CondBr(bit, move, nil, join, nil)
	g.blk = move
	g.hop(where)
	g.blk.Br(join)
	g.blk = join
}

// there is where the function being lowered runs: the main executor for
// @MainActor code, the task's home for the rest, as Swift runs
// nonisolated async code on the generic executor and not on whatever
// actor called it.
func (g *gen) there() int64 {
	if g.isolated() {
		return hopMain
	}
	return hopHome
}

// prologueHop starts an async function's body where it runs. A
// nonisolated one that never suspends is left where its caller was:
// nothing in it can tell, and the hop would give a function that needed
// no frame one. The proposal calls this the one optimization on Swift's
// rule that is safe to take first.
func (g *gen) prologueHop(body []ast.Stmt) {
	if !g.isolated() && !analyzer.Awaits(body) {
		return
	}
	g.ensure(g.there())
}

// await lowers `await x`: x, and then the hop back to where this
// function runs, since whatever x called may have left the task
// elsewhere -- Swift's hop_to_executor after every suspension. A
// synchronous @MainActor call or property awaited from nonisolated code
// is the one case where the await itself is the hop: to the main thread
// first, and back after.
func (g *gen) await(n *ast.AwaitExpr) *sil.Value {
	if !g.isolated() && g.awaitsIsolatedSync(n.X) {
		g.ensure(hopMain)
	}
	v := g.expr(n.X)
	g.ensure(g.there())
	return v
}

// awaitsIsolatedSync reports whether an awaited expression is a call to
// a synchronous @MainActor function, or a read of a @MainActor property:
// what runs only on the main thread and cannot get there itself.
func (g *gen) awaitsIsolatedSync(x ast.Expr) bool {
	switch e := x.(type) {
	case *ast.ParenExpr:
		return g.awaitsIsolatedSync(e.X)
	case *ast.TryExpr:
		return g.awaitsIsolatedSync(e.X)
	case *ast.CallExpr:
		var sig *types.Signature
		if t := g.typeOf(e.Fun); t != nil {
			sig, _ = t.Underlying().(*types.Signature)
		}
		if mem, ok := e.Fun.(*ast.MemberExpr); ok && sig == nil {
			if ref := g.info.Methods[mem]; ref != nil && ref.Method != nil {
				sig = ref.Method.Sig
			}
		}
		if sig == nil {
			if meta, ok := g.typeOf(e.Fun).(*types.Metatype); ok {
				return g.info.MainActor[meta.Instance.Underlying()]
			}
			return false
		}
		return sig.Isolated && !sig.Async
	case *ast.MemberExpr:
		if e.Name == nil {
			return false
		}
		return analyzer.IsolatedField(g.typeOf(e.X), g.text(e.Name))
	case *ast.IdentExpr:
		if v, ok := g.info.Uses[e.Name].(*analyzer.VarSymbol); ok {
			return v.Isolated()
		}
	}
	return false
}
