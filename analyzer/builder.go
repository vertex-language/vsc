package analyzer

import (
	"strconv"

	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/token"
	"github.com/vertex-language/vsc/types"
)

// Result builders (SE-0289).
//
// A closure passed where a parameter is marked with a result builder --
// `@Lines _ body: (Bool) -> [String]` -- is not run as written: each of
// its statements is a part the builder combines. The closure is rewritten
// in place, once, into the calls Swift makes of the builder, and then
// checked and lowered as any closure is:
//
//	"title"                  let $b0 = Lines.buildExpression("title")
//	if v { "x" }             let $b1 = Lines.buildOptional({ if v { ...; return Lines.buildBlock(...) }; return nil }())
//	if v { "a" } else { "b" }  let $b2 = { if v { return Lines.buildEither(first: ...) } else { return Lines.buildEither(second: ...) } }()
//	for i in xs { "i" }      let $b3 = Lines.buildArray(xs.map { i in ...; return Lines.buildBlock(...) })
//	                         return Lines.buildBlock($b0, $b1, $b2, $b3)
//
// A declaration in the body stays a declaration, and parts nothing
// builds are refused where the builder lacks the method.

// builderOf is the result builder a parameter's attributes name, or nil.
func (c *checker) builderOf(attrs []*ast.Attr, scope *Scope) types.Type {
	for _, a := range attrs {
		if a == nil {
			continue
		}
		id, ok := a.Name.(*ast.IdentType)
		if !ok || id.Name == nil {
			continue
		}
		tn, ok := scope.Lookup(id.Name.Text(c.file)).(*TypeNameSymbol)
		if !ok || tn.Type() == nil {
			continue
		}
		// Whether it is a builder -- declares buildBlock -- is asked
		// where a closure is given for the parameter: a signature is
		// read before the types' members are.
		return tn.Type()
	}
	return nil
}

// builderHas reports whether a builder declares the static method named.
func builderHas(builder types.Type, name string) bool {
	_, ms := methodsNamed(&types.Metatype{Instance: builder}, name)
	return len(ms) > 0
}

// builderRewrite is one closure's rewriting: the builder, the name the
// source spells it with, and the parts numbered so far.
type builderRewrite struct {
	c       *checker
	builder types.Type
	name    string
	n       int
}

// applyBuilder rewrites a closure for the builder, if it has not been.
func (c *checker) applyBuilder(x ast.Expr, builder types.Type) {
	cl, ok := unparen(x).(*ast.ClosureExpr)
	if !ok || builder == nil || !builderHas(builder, "buildBlock") {
		return
	}
	if c.builtClosures == nil {
		c.builtClosures = map[*ast.ClosureExpr]bool{}
	}
	if c.builtClosures[cl] {
		return
	}
	c.builtClosures[cl] = true
	r := &builderRewrite{c: c, builder: builder, name: builder.String()}
	cl.Stmts = r.block(cl.Stmts, ast.Span{Lo: cl.Pos(), Hi: cl.End()}, nil)
}

// block is statements rewritten into parts, and a return of what
// buildBlock makes of them -- wrapped by wrap, where one is given, as
// buildEither wraps a branch.
func (r *builderRewrite) block(stmts []ast.Stmt, at ast.Span, wrap func(ast.Expr) ast.Expr) []ast.Stmt {
	var out []ast.Stmt
	var parts []ast.Expr
	for _, s := range stmts {
		part, keep := r.part(s)
		if keep != nil {
			out = append(out, keep)
		}
		if part == nil {
			continue
		}
		name := "$builder" + strconv.Itoa(r.n)
		r.n++
		sp := ast.Span{Lo: s.Pos(), Hi: s.End()}
		out = append(out, letStmt(name, part, sp))
		parts = append(parts, ident(name, sp))
	}
	var result ast.Expr = r.call("buildBlock", at, parts...)
	if wrap != nil {
		result = wrap(result)
	}
	return append(out, &ast.ReturnStmt{Span: at, Return: at.Lo, X: result})
}

// part is what a statement contributes: an expression that makes its
// part, or a statement kept as it is.
func (r *builderRewrite) part(s ast.Stmt) (ast.Expr, ast.Stmt) {
	sp := ast.Span{Lo: s.Pos(), Hi: s.End()}
	switch x := s.(type) {
	case *ast.ExprStmt:
		if builderHas(r.builder, "buildExpression") {
			return r.call("buildExpression", sp, x.X), nil
		}
		return x.X, nil
	case *ast.IfStmt:
		if x.Else == nil {
			// buildOptional of the block, or nil where the condition
			// does not hold.
			inner := &ast.IfStmt{Span: x.Span, If: x.If, Conds: x.Conds,
				Body: &ast.CodeBlock{Span: x.Body.Span, Lbrace: x.Body.Lbrace, Rbrace: x.Body.Rbrace,
					Stmts: r.block(x.Body.Stmts, x.Body.Span, nil)}}
			nilLit := &ast.BasicLit{Span: ast.Span{Lo: sp.Hi, Hi: sp.Hi}, Kind: token.NIL}
			body := []ast.Stmt{inner, &ast.ReturnStmt{Span: sp, Return: sp.Lo, X: nilLit}}
			return r.call("buildOptional", sp, immediate(body, sp)), nil
		}
		return immediate([]ast.Stmt{r.either(x)}, sp), nil
	case *ast.ForInStmt:
		// buildArray of what each pass builds.
		elem := "$builderElement" + strconv.Itoa(r.n)
		r.n++
		stmts := []ast.Stmt{letPattern(x.Pat, ident(elem, sp), sp)}
		stmts = append(stmts, r.block(x.Body.Stmts, x.Body.Span, nil)...)
		each := &ast.ClosureExpr{Span: x.Body.Span, Lbrace: x.Body.Lbrace, Rbrace: x.Body.Rbrace,
			Sig: &ast.ClosureSig{Span: sp, Params: &ast.ClosureParams{Span: sp,
				Params: []*ast.ClosureParam{{Span: sp, Name: &ast.Ident{Span: sp, Synth: elem}}}}},
			Stmts: stmts}
		seq := x.Seq
		mapped := &ast.CallExpr{Span: sp,
			Fun:  &ast.MemberExpr{Span: sp, X: &ast.ParenExpr{Span: ast.Span{Lo: seq.Pos(), Hi: seq.End()}, X: seq}, Dot: seq.End(), Name: &ast.Ident{Span: sp, Synth: "map"}},
			Args: &ast.CallArgs{Span: sp, Args: []*ast.CallArg{{Span: sp, X: each}}}}
		return r.call("buildArray", sp, mapped), nil
	}
	return nil, s
}

// either is an if with an else, each branch returning its block wrapped
// in buildEither(first:) or buildEither(second:); an else-if nests.
func (r *builderRewrite) either(x *ast.IfStmt) ast.Stmt {
	first := func(e ast.Expr) ast.Expr { return r.labelled("buildEither", "first", e) }
	second := func(e ast.Expr) ast.Expr { return r.labelled("buildEither", "second", e) }
	out := &ast.IfStmt{Span: x.Span, If: x.If, Conds: x.Conds, ElsePos: x.ElsePos,
		Body: &ast.CodeBlock{Span: x.Body.Span, Lbrace: x.Body.Lbrace, Rbrace: x.Body.Rbrace,
			Stmts: r.block(x.Body.Stmts, x.Body.Span, first)}}
	switch el := x.Else.(type) {
	case *ast.CodeBlock:
		out.Else = &ast.CodeBlock{Span: el.Span, Lbrace: el.Lbrace, Rbrace: el.Rbrace,
			Stmts: r.block(el.Stmts, el.Span, second)}
	case *ast.IfStmt:
		sp := ast.Span{Lo: el.Pos(), Hi: el.End()}
		var inner ast.Expr
		if el.Else == nil {
			part, _ := r.part(el)
			inner = part
		} else {
			inner = immediate([]ast.Stmt{r.either(el)}, sp)
		}
		out.Else = &ast.CodeBlock{Span: sp, Lbrace: sp.Lo, Rbrace: sp.Hi,
			Stmts: []ast.Stmt{&ast.ReturnStmt{Span: sp, Return: sp.Lo, X: second(inner)}}}
	}
	return out
}

// call is `Builder.name(args...)`.
func (r *builderRewrite) call(name string, at ast.Span, args ...ast.Expr) ast.Expr {
	fun := &ast.MemberExpr{Span: at, X: ident(r.name, at), Dot: at.Lo, Name: &ast.Ident{Span: at, Synth: name}}
	out := &ast.CallExpr{Span: at, Fun: fun, Args: &ast.CallArgs{Span: at}}
	for _, a := range args {
		out.Args.Args = append(out.Args.Args, &ast.CallArg{Span: ast.Span{Lo: a.Pos(), Hi: a.End()}, X: a})
	}
	return out
}

// labelled is `Builder.name(label: arg)`.
func (r *builderRewrite) labelled(name, label string, arg ast.Expr) ast.Expr {
	at := ast.Span{Lo: arg.Pos(), Hi: arg.End()}
	out := r.call(name, at, arg).(*ast.CallExpr)
	out.Args.Args[0].Label = &ast.Ident{Span: at, Synth: label}
	return out
}

// immediate is `{ body }()`: statements as an expression, whose result
// type comes from where it is used or from its returns.
func immediate(body []ast.Stmt, at ast.Span) ast.Expr {
	cl := &ast.ClosureExpr{Span: at, Lbrace: at.Lo, Rbrace: at.Hi, Stmts: body}
	return &ast.CallExpr{Span: at, Fun: cl, Args: &ast.CallArgs{Span: at}}
}

// ident is a synthesized name used as an expression.
func ident(name string, at ast.Span) ast.Expr {
	return &ast.IdentExpr{Span: at, Name: &ast.Ident{Span: at, Synth: name}}
}

// letStmt is `let name = value`.
func letStmt(name string, value ast.Expr, at ast.Span) ast.Stmt {
	return letPattern(&ast.IdentPattern{Span: at, Name: &ast.Ident{Span: at, Synth: name}}, value, at)
}

// letPattern is `let pat = value`.
func letPattern(pat ast.Pattern, value ast.Expr, at ast.Span) ast.Stmt {
	return &ast.DeclStmt{Span: at, D: &ast.VarDecl{Span: at, Keyword: at.Lo, Kind: token.LET,
		Bindings: []*ast.PatternBinding{{Span: at, Pat: pat, Assign: at.Lo, Value: value}}}}
}

// dynamicMember reads `x.name` on a @dynamicMemberLookup type that has no
// member of that name as `x[dynamicMember: "name"]`, or, where its
// subscript takes a key path, `x[dynamicMember: \.name]` (SE-0195,
// SE-0252). The subscript is what is checked and lowered.
func (c *checker) dynamicMember(e *ast.MemberExpr, base types.Type, expected types.Type, scope *Scope) (types.Type, bool) {
	if base == nil || isInvalid(base) || e.Name == nil {
		return nil, false
	}
	decl := base
	if gi, ok := decl.(*types.GenericInstance); ok {
		decl = gi.Base
	}
	if !c.info.DynamicMembers[decl] {
		return nil, false
	}
	var subs []*types.Subscript
	switch u := decl.Underlying().(type) {
	case *types.Struct:
		subs = u.Subscripts
	case *types.Class:
		subs = u.Subscripts
	case *types.Enum:
		subs = u.Subscripts
	}
	at := ast.Span{Lo: e.Name.Pos(), Hi: e.Name.End()}
	var key ast.Expr
	for _, s := range subs {
		if s == nil || len(s.Params) != 1 || s.Params[0].Label != "dynamicMember" {
			continue
		}
		if isString(s.Params[0].Type) {
			key = &ast.StringLit{Span: at, Open: at.Lo, Close: at.Hi,
				Segments: []ast.Node{&ast.StringText{Span: at}}}
			break
		}
		key = &ast.KeyPathExpr{Span: at, Backslash: at.Lo,
			Components: []*ast.KeyPathComponent{{Span: at, Name: e.Name}}}
	}
	if key == nil {
		return nil, false
	}
	span := ast.Span{Lo: e.Pos(), Hi: e.End()}
	sub := &ast.SubscriptExpr{Span: span, X: e.X, Lsquare: at.Lo, Rsquare: at.Hi,
		Args: []*ast.CallArg{{Span: at, Label: &ast.Ident{Span: at, Synth: "dynamicMember"}, X: key}}}
	t := c.checkExpr(sub, expected, scope)
	c.info.ImplicitSelf[e] = sub
	return t, true
}

// callAsFunction checks `x(args)` as `x.callAsFunction(args)` where x is a
// value of a type that declares one.
func (c *checker) callAsFunction(e *ast.CallExpr, callee types.Type, expected types.Type, scope *Scope) (types.Type, bool) {
	if callee == nil || isInvalid(callee) {
		return nil, false
	}
	switch callee.Underlying().(type) {
	case *types.Struct, *types.Class, *types.Enum:
	default:
		return nil, false
	}
	if _, m := c.findMethod(callee, "callAsFunction"); m == nil {
		return nil, false
	}
	at := ast.Span{Lo: e.Fun.Pos(), Hi: e.Fun.End()}
	call := &ast.CallExpr{Span: e.Span, Args: e.Args, Trailing: e.Trailing,
		Fun: &ast.MemberExpr{Span: at, X: e.Fun, Dot: at.Hi, Name: &ast.Ident{Span: at, Synth: "callAsFunction"}}}
	t := c.checkExpr(call, expected, scope)
	c.info.ImplicitSelf[e] = call
	return t, true
}
