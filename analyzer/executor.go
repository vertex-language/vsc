package analyzer

import "github.com/vertex-language/vsc/ast"

// rewriteExecutorPreference makes `Task(executorPreference: e) { body }`
// and `Task.detached(executorPreference: e) { body }` the Task without the
// argument, whose operation takes the preference up first:
//
//	Task { await _vertexAdoptPreference(e); body }
//
// The task starts where Task {} would and moves to e before anything of
// body runs, which is what SE-0417's preference asks: body's code isolated
// to no actor runs on e. e is read by the task, once, as it starts.
//
// It reports whether it rewrote the call. The rest of the checker then
// sees the Task it knows.
func (c *checker) rewriteExecutorPreference(e *ast.CallExpr, scope *Scope) bool {
	if e.Args == nil || len(e.Args.Args) < 2 || !c.namesCoreTask(e.Fun, scope) {
		return false
	}
	at := -1
	for i, a := range e.Args.Args {
		if a.Label != nil && a.Label.Text(c.file) == "executorPreference" {
			at = i
		}
	}
	if at < 0 {
		return false
	}
	last := e.Args.Args[len(e.Args.Args)-1]
	cl, ok := unparen(last.X).(*ast.ClosureExpr)
	if !ok || at == len(e.Args.Args)-1 {
		return false
	}
	pref := e.Args.Args[at].X
	span := ast.Span{Lo: pref.Pos(), Hi: pref.End()}
	adopt := &ast.CallExpr{Span: span,
		Fun:  &ast.IdentExpr{Span: span, Name: &ast.Ident{Span: span, Synth: "_vertexAdoptPreference"}},
		Args: &ast.CallArgs{Span: span, Args: []*ast.CallArg{{Span: span, X: pref}}}}
	first := &ast.ExprStmt{Span: span, X: &ast.AwaitExpr{Span: span, Await: pref.Pos(), X: adopt}}
	// A body of one expression is what the operation returns; with a
	// statement before it, it has to say so.
	stmts := cl.Stmts
	if len(stmts) == 1 {
		if es, isExpr := stmts[0].(*ast.ExprStmt); isExpr {
			stmts = []ast.Stmt{&ast.ReturnStmt{Span: es.Span, Return: es.Pos(), X: es.X}}
		}
	}
	cl.Stmts = append([]ast.Stmt{first}, stmts...)
	e.Args.Args = append(e.Args.Args[:at:at], e.Args.Args[at+1:]...)
	return true
}

// namesCoreTask reports whether fun is `Task` or `Task.detached`, and
// Task the core's.
func (c *checker) namesCoreTask(fun ast.Expr, scope *Scope) bool {
	switch f := fun.(type) {
	case *ast.IdentExpr:
		if f.Name == nil || f.Name.Text(c.file) != "Task" {
			return false
		}
		tn, ok := c.lookupValue(scope, "Task").(*TypeNameSymbol)
		return ok && c.isCoreTask(tn.Type())
	case *ast.MemberExpr:
		if f.Name == nil || f.Name.Text(c.file) != "detached" {
			return false
		}
		return c.namesCoreTask(f.X, scope)
	}
	return false
}

// coreGlobalRead is a read of one of the core's module-level variables
// made a call of the function the core reads it with, `_vertexGlobal_x()`
// for `x`, where the core declares one; nil otherwise. A module lowers
// only its own module-level variables and the core's functions it calls,
// so a core variable a program reads -- globalConcurrentExecutor -- is
// read through a function.
func (c *checker) coreGlobalRead(expr ast.Expr, scope *Scope) ast.Expr {
	id, ok := expr.(*ast.IdentExpr)
	if !ok || id.Name == nil || id.Args != nil {
		return nil
	}
	core := c.modules["Swift"]
	if core == nil {
		return nil
	}
	name := id.Name.Text(c.file)
	found, sym := scope.LookupParent(name)
	if _, isVar := sym.(*VarSymbol); !isVar || found != core {
		return nil
	}
	reader := "_vertexGlobal_" + name
	if _, isFunc := core.Lookup(reader).(*FuncSymbol); !isFunc {
		return nil
	}
	return &ast.CallExpr{Span: id.Span,
		Fun:  &ast.IdentExpr{Span: id.Span, Name: &ast.Ident{Span: id.Span, Synth: reader}},
		Args: &ast.CallArgs{Span: id.Span}}
}
