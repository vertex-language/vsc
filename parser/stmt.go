package parser

import (
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/token"
)

// stop is what ends a statement list: the list runs to end of file
// unless one of these says otherwise.
type stop uint

const (
	stopEOF   stop = 0
	stopBrace stop = 1 << iota // a '}' closes the list
	stopCase                   // a case or default label closes it — a switch body
	stopPound                  // an #elseif, #else or #endif closes it
)

func (p *parser) atStop(s stop) bool {
	switch p.kind() {
	case token.EOF:
		return true
	case token.RBRACE:
		return s&stopBrace != 0
	case token.CASE, token.DEFAULT:
		return s&stopCase != 0
	case token.AT:
		// `@unknown default:` — a case label with an attribute on it.
		return s&stopCase != 0 && p.atLabelAfterAttrs()
	case token.POUND_ELSEIF, token.POUND_ELSE, token.POUND_ENDIF:
		return s&stopPound != 0
	case token.POUND_IF:
		// A #if in a switch body holds either statements, which
		// belong to the case being read, or case labels, which do
		// not: the switch reads that one itself.
		return s&stopCase != 0 && p.atCaseAfterPound()
	}
	return false
}

// atCaseAfterPound reports whether the #if at the cursor opens a
// conditional case label. Its condition runs to the end of the line,
// so what follows the line is what the directive is made of.
func (p *parser) atCaseAfterPound() bool {
	n := 1
	for !p.peekTok(n).Flags.Has(token.FlagNLBefore) && p.peek(n) != token.EOF {
		n++
	}
	switch k := p.peek(p.skipAttrsAt(n)); k {
	case token.CASE, token.DEFAULT:
		return true
	}
	return false
}

// parseStmtList reads Statements. A statement ends at a newline or a
// semicolon, so two statements written on one line without one
// between them is a mistake worth naming — the second is parsed
// anyway.
func (p *parser) parseStmtList(s stop) []ast.Stmt {
	var out []ast.Stmt
	for !p.atStop(s) {
		start := p.i
		st := p.parseStmt()
		out = append(out, st)

		if p.at(token.SEMI) {
			semi := p.pos()
			p.next()
			_ = semi
		} else if !p.atStop(s) && !p.nl() && p.i != start {
			p.errHere("consecutive statements on a line must be separated by ';'")
		}

		if p.i == start { // progress check: force a resync
			p.advanceTo(stmtFollow)
			if p.at(token.SEMI) {
				p.next()
			}
			if p.i == start {
				p.next()
			}
		}
	}
	return out
}

func (p *parser) parseStmt() ast.Stmt {
	p.depth++
	defer func() { p.depth-- }()
	lo := p.pos()
	if p.tooDeep() {
		return &ast.BadStmt{Span: p.span(lo)}
	}

	switch p.kind() {
	case token.SEMI:
		semi := p.pos()
		p.next()
		return &ast.EmptyStmt{Span: p.span(lo), Semi: semi}

	case token.FOR:
		return p.parseForIn()
	case token.WHILE:
		return p.parseWhile()
	case token.REPEAT:
		// `repeat { … } while c` is the loop; `repeat each x` is a
		// pack expansion, which is an expression and can stand as a
		// statement. The brace is what tells them apart.
		if p.peek(1) == token.LBRACE {
			return p.parseRepeatWhile()
		}
	case token.IF:
		return p.parseIf()
	case token.GUARD:
		return p.parseGuard()
	case token.SWITCH:
		return p.parseSwitch()
	case token.DO:
		return p.parseDo()
	case token.DEFER:
		return p.parseDefer()
	case token.BREAK, token.CONTINUE:
		return p.parseBreakContinue()
	case token.FALLTHROUGH:
		kw := p.pos()
		p.next()
		return &ast.FallthroughStmt{Span: p.span(lo), Keyword: kw}
	case token.RETURN:
		return p.parseReturn()
	case token.THROW:
		kw := p.pos()
		p.next()
		x := p.parseExpr(0)
		return &ast.ThrowStmt{Span: p.span(lo), Throw: kw, X: x}

	case token.POUND_IF:
		return p.parseIfConfig(stopPound)
	case token.POUND_SOURCELOCATION:
		return p.parseSourceLocation()
	case token.POUND_ERROR, token.POUND_WARNING:
		return p.parseDiagnostic()

	case token.IDENT:
		switch {
		case p.peek(1) == token.COLON && labelable(p.peek(2)):
			return p.parseLabeled()
		case p.inCoroutine && p.atWord("yield") && !p.peekTok(1).Flags.Has(token.FlagNLBefore):
			kw := p.pos()
			p.next()
			y := &ast.YieldStmt{Keyword: kw}
			if p.at(token.AMP_PREFIX) {
				y.Amp = p.pos()
				p.next()
			}
			y.X = p.parseExpr(0)
			y.Span = p.span(lo)
			return y
		case p.atWord("discard") && p.atDiscard():
			kw := p.pos()
			p.next()
			x := p.parseExpr(0)
			return &ast.DiscardStmt{Span: p.span(lo), Keyword: kw, X: x}
		}
	}

	if p.atDeclStart() {
		d := p.parseDecl()
		return &ast.DeclStmt{Span: ast.Span{Lo: d.Pos(), Hi: d.End()}, D: d}
	}

	x := p.parseExpr(0)
	return &ast.ExprStmt{Span: ast.Span{Lo: x.Pos(), Hi: x.End()}, X: x}
}

// atLabelAfterAttrs reports whether the attributes at the cursor are
// written on a case label rather than on a declaration or a closure.
func (p *parser) atLabelAfterAttrs() bool {
	k := p.peek(p.skipAttrsAt(0))
	return k == token.CASE || k == token.DEFAULT
}

// labelable is the set of statements a StatementLabel may precede.
func labelable(k token.Kind) bool {
	switch k {
	case token.FOR, token.WHILE, token.REPEAT, token.IF, token.SWITCH, token.DO:
		return true
	}
	return false
}

// atDiscard settles `discard` the statement from `discard` the name.
// The statement's operand is the value being discarded — `discard
// self`, `discard x` — so a following operator, assignment, or
// anything that would make this a use of the name instead is not one.
func (p *parser) atDiscard() bool {
	t := p.peekTok(1)
	if t.Flags.Has(token.FlagNLBefore) {
		return false
	}
	return t.Kind == token.SELF || t.Kind == token.IDENT
}

func (p *parser) parseLabeled() ast.Stmt {
	lo := p.pos()
	label := p.ident()
	colon := p.expect(token.COLON)
	st := p.parseStmt()
	return &ast.LabeledStmt{Span: p.span(lo), Label: label, Colon: colon, Stmt: st}
}

// ---- loops ----

func (p *parser) parseForIn() ast.Stmt {
	lo := p.pos()
	kw := p.pos()
	p.next()

	s := &ast.ForInStmt{For: kw}
	s.Await = p.takeWord("await")
	if p.at(token.CASE) {
		s.Case = p.pos()
		p.next()
	}
	s.Pat = p.parsePattern(patternMatching)
	s.In = p.expect(token.IN)
	s.Seq = p.parseExpr(exprBasic)
	s.Where = p.parseWhereClause()
	s.Body = p.parseCodeBlock()
	s.Span = p.span(lo)
	return s
}

func (p *parser) parseWhereClause() *ast.WhereClause {
	if !p.at(token.WHERE) {
		return nil
	}
	lo := p.pos()
	kw := p.pos()
	p.next()
	cond := p.parseExpr(exprBasic)
	return &ast.WhereClause{Span: p.span(lo), Where: kw, Cond: cond}
}

func (p *parser) parseWhile() ast.Stmt {
	lo := p.pos()
	kw := p.pos()
	p.next()
	conds := p.parseConditions()
	body := p.parseCodeBlock()
	return &ast.WhileStmt{Span: p.span(lo), While: kw, Conds: conds, Body: body}
}

func (p *parser) parseRepeatWhile() ast.Stmt {
	lo := p.pos()
	kw := p.pos()
	p.next()
	body := p.parseCodeBlock()
	while := p.expect(token.WHILE)
	cond := p.parseExpr(0)
	return &ast.RepeatWhileStmt{Span: p.span(lo), Repeat: kw, Body: body, While: while, Cond: cond}
}

// ---- branches ----

func (p *parser) parseIf() ast.Stmt {
	lo := p.pos()
	kw := p.pos()
	p.next()
	s := &ast.IfStmt{If: kw}
	s.Conds = p.parseConditions()
	s.Body = p.parseCodeBlock()
	if p.at(token.ELSE) {
		s.ElsePos = p.pos()
		p.next()
		if p.at(token.IF) {
			s.Else = p.parseIf()
		} else {
			s.Else = p.parseCodeBlock()
		}
	}
	s.Span = p.span(lo)
	return s
}

func (p *parser) parseGuard() ast.Stmt {
	lo := p.pos()
	kw := p.pos()
	p.next()
	conds := p.parseConditions()
	elsePos := p.expect(token.ELSE)
	body := p.parseCodeBlock()
	return &ast.GuardStmt{Span: p.span(lo), Guard: kw, Conds: conds, ElsePos: elsePos, Body: body}
}

func (p *parser) parseSwitch() ast.Stmt {
	lo := p.pos()
	kw := p.pos()
	p.next()
	s := &ast.SwitchStmt{Switch: kw}
	s.Subject = p.parseExpr(exprBasic)
	s.Lbrace = p.expect(token.LBRACE)
	for !p.at(token.RBRACE) && !p.at(token.EOF) {
		start := p.i
		switch {
		case p.at(token.CASE), p.at(token.DEFAULT), p.at(token.AT):
			s.Cases = append(s.Cases, p.parseCaseClause())
		case p.at(token.POUND_IF):
			s.Cases = append(s.Cases, p.parseIfConfigCases())
		default:
			p.errHere("expected 'case' or 'default' in a switch body")
			p.advanceTo(map[token.Kind]bool{token.CASE: true, token.DEFAULT: true, token.RBRACE: true})
		}
		if p.i == start {
			p.next()
		}
	}
	s.Rbrace = p.expect(token.RBRACE)
	s.Span = p.span(lo)
	return s
}

func (p *parser) parseCaseClause() ast.Stmt {
	lo := p.pos()
	c := &ast.CaseClause{Attrs: p.parseAttrs()}
	c.Keyword, c.Kind = p.pos(), p.kind()
	switch {
	case p.at(token.CASE):
		p.next()
		for {
			item := &ast.CaseItem{}
			ilo := p.pos()
			item.Pat = p.parsePattern(patternMatching)
			item.Where = p.parseWhereClause()
			item.Span = p.span(ilo)
			c.Items = append(c.Items, item)
			if !p.at(token.COMMA) {
				break
			}
			p.next()
		}
	case p.at(token.DEFAULT):
		p.next()
	default:
		p.errHere("expected 'case' or 'default' after an attribute in a switch body")
		c.Kind = token.CASE
	}
	c.Colon = p.expect(token.COLON)
	c.Stmts = p.parseStmtList(stopBrace | stopCase | stopPound)
	if len(c.Stmts) == 0 {
		p.errHere("a case must have at least one statement; write 'break' for one that does nothing")
	}
	c.Span = p.span(lo)
	return c
}

// parseIfConfigCases reads a ConditionalSwitchCase: a conditional
// compilation block whose clauses hold case labels rather than
// statements. The grammar names the production without spelling it
// out; this is what it can only mean.
func (p *parser) parseIfConfigCases() ast.Stmt {
	lo := p.pos()
	s := &ast.IfConfigStmt{}
	for {
		clo := p.pos()
		cl := &ast.IfConfigClause{Pound: p.pos(), Kind: p.kind()}
		p.next()
		if cl.Kind != token.POUND_ELSE {
			cl.Cond = p.parseCompilationCond()
		}
		for p.at(token.CASE) || p.at(token.DEFAULT) || p.at(token.AT) {
			cl.Stmts = append(cl.Stmts, p.parseCaseClause())
		}
		cl.Span = p.span(clo)
		s.Clauses = append(s.Clauses, cl)
		if !p.at(token.POUND_ELSEIF) && !p.at(token.POUND_ELSE) {
			break
		}
	}
	s.Endif = p.expect(token.POUND_ENDIF)
	s.Span = p.span(lo)
	return s
}

// ---- conditions ----

// parseConditions reads a ConditionList: the head of an if, a guard,
// or a while.
func (p *parser) parseConditions() []ast.Node {
	var out []ast.Node
	for {
		out = append(out, p.parseCondition())
		if !p.at(token.COMMA) {
			break
		}
		p.next()
	}
	return out
}

func (p *parser) parseCondition() ast.Node {
	lo := p.pos()
	switch {
	case p.at(token.POUND_AVAILABLE), p.at(token.POUND_UNAVAILABLE):
		return p.parseAvailability()

	case p.at(token.CASE):
		kw := p.pos()
		p.next()
		pat := p.parsePattern(patternMatching)
		c := &ast.CaseCond{Case: kw, Pat: pat}
		if p.at(token.ASSIGN) {
			c.Assign = p.pos()
			p.next()
			c.Value = p.parseExpr(exprBasic)
		} else {
			p.errHere("expected '=' after a 'case' condition's pattern")
		}
		c.Span = p.span(lo)
		return c

	case p.at(token.LET), p.at(token.VAR):
		b := &ast.OptionalBinding{Keyword: p.pos(), Kind: p.kind()}
		p.next()
		b.Pat = p.parsePattern(patternBinding)
		if p.at(token.ASSIGN) {
			b.Assign = p.pos()
			p.next()
			b.Value = p.parseExpr(exprBasic)
		}
		b.Span = p.span(lo)
		return b
	}
	return p.parseExpr(exprBasic)
}

// parseAvailability reads #available ( … ) or #unavailable ( … ).
func (p *parser) parseAvailability() ast.Node {
	lo := p.pos()
	c := &ast.AvailabilityCond{Pound: p.pos(), Kind: p.kind()}
	p.next()
	c.Lparen = p.expect(token.LPAREN)
	for !p.at(token.RPAREN) && !p.at(token.EOF) {
		start := p.i
		alo := p.pos()
		arg := &ast.AvailabilityArg{}
		switch {
		case p.atOper("*"):
			arg.Star = p.pos()
			p.next()
		case p.at(token.IDENT):
			arg.Platform = p.ident()
			if p.at(token.INT_LIT) || p.at(token.FLOAT_LIT) {
				arg.Version = p.parseVersion()
			}
		default:
			p.errHere("expected a platform name or '*'")
		}
		arg.Span = p.span(alo)
		c.Args = append(c.Args, arg)
		if !p.at(token.COMMA) {
			break
		}
		p.next()
		if p.i == start {
			p.next()
		}
	}
	c.Rparen = p.expect(token.RPAREN)
	c.Span = p.span(lo)
	return c
}

// parseVersion reads a PlatformVersion: DecimalDigit {. DecimalDigit}.
// It is not a number — 15.10 is not 15.1 — so the digits and dots are
// read as one span whatever the scanner made of them.
// parseCondVersion reads the version a compilation condition
// compares against, written either as digits and dots or as a string.
func (p *parser) parseCondVersion(c *ast.PlatformCond) {
	switch {
	case p.at(token.INT_LIT) || p.at(token.FLOAT_LIT):
		c.Ver = p.parseVersion()
	case p.at(token.STRING_QUOTE) || p.at(token.MULTILINE_STRING_QUOTE) ||
		p.at(token.POUND_DELIM):
		c.VerStr = p.parseStringLit()
	default:
		p.errHere("expected a version number")
	}
}

func (p *parser) parseVersion() *ast.VersionLit {
	lo := p.pos()
	p.next() // the first INT_LIT or FLOAT_LIT
	for p.at(token.PERIOD) &&
		(p.peek(1) == token.INT_LIT || p.peek(1) == token.FLOAT_LIT) {
		p.next()
		p.next()
	}
	return &ast.VersionLit{Span: p.span(lo)}
}

// ---- do, defer, and control transfer ----

func (p *parser) parseDo() ast.Stmt {
	lo := p.pos()
	kw := p.pos()
	p.next()
	s := &ast.DoStmt{Do: kw}
	s.Throws = p.parseThrowsClause()
	s.Body = p.parseCodeBlock()
	for p.at(token.CATCH) {
		clo := p.pos()
		c := &ast.CatchClause{Catch: p.pos()}
		p.next()
		if !p.at(token.LBRACE) {
			for {
				ilo := p.pos()
				item := &ast.CaseItem{Pat: p.parsePattern(patternMatching)}
				item.Where = p.parseWhereClause()
				item.Span = p.span(ilo)
				c.Items = append(c.Items, item)
				if !p.at(token.COMMA) {
					break
				}
				p.next()
			}
		}
		c.Body = p.parseCodeBlock()
		c.Span = p.span(clo)
		s.Catches = append(s.Catches, c)
	}
	s.Span = p.span(lo)
	return s
}

func (p *parser) parseDefer() ast.Stmt {
	lo := p.pos()
	kw := p.pos()
	p.next()
	body := p.parseCodeBlock()
	return &ast.DeferStmt{Span: p.span(lo), Defer: kw, Body: body}
}

func (p *parser) parseBreakContinue() ast.Stmt {
	lo := p.pos()
	kw, kind := p.pos(), p.kind()
	p.next()
	var label *ast.Ident
	if p.at(token.IDENT) && !p.nl() {
		label = p.ident()
	}
	if kind == token.BREAK {
		return &ast.BreakStmt{Span: p.span(lo), Break: kw, Label: label}
	}
	return &ast.ContinueStmt{Span: p.span(lo), Continue: kw, Label: label}
}

// parseReturn reads return [Expression]. The expression is optional,
// and a newline is what says it was omitted: `return` on its own line
// returns nothing, whatever follows it.
func (p *parser) parseReturn() ast.Stmt {
	lo := p.pos()
	kw := p.pos()
	p.next()
	var x ast.Expr
	if !p.nl() && p.atExprStart() {
		x = p.parseExpr(0)
	}
	return &ast.ReturnStmt{Span: p.span(lo), Return: kw, X: x}
}

// ---- compiler control statements ----

// parseIfConfig reads a ConditionalCompilationBlock. Every clause's
// body is parsed, whether its condition holds or not: this is a
// statement in the grammar, not a preprocessor, so the tree records
// what was written and the analyzer decides what counts.
func (p *parser) parseIfConfig(s stop) ast.Stmt {
	lo := p.pos()
	blk := &ast.IfConfigStmt{}
	for {
		clo := p.pos()
		cl := &ast.IfConfigClause{Pound: p.pos(), Kind: p.kind()}
		p.next()
		if cl.Kind != token.POUND_ELSE {
			cl.Cond = p.parseCompilationCond()
		}
		cl.Stmts = p.parseStmtList(s | stopPound)
		cl.Span = p.span(clo)
		blk.Clauses = append(blk.Clauses, cl)
		if !p.at(token.POUND_ELSEIF) && !p.at(token.POUND_ELSE) {
			break
		}
	}
	blk.Endif = p.expect(token.POUND_ENDIF)
	blk.Span = p.span(lo)
	return blk
}

// parseCompilationCond reads a CompilationCondition. Its operators
// are && and ||, and it is left flat like any other sequence: the
// analyzer that evaluates it folds it the same way.
func (p *parser) parseCompilationCond() ast.Expr {
	lo := p.pos()
	first := p.parseCompilationOperand()
	var elems []ast.Expr
	for p.atOper("&&") || p.atOper("||") {
		if elems == nil {
			elems = []ast.Expr{first}
		}
		elems = append(elems, p.oper(), p.parseCompilationOperand())
	}
	if elems == nil {
		return first
	}
	return &ast.SequenceExpr{Span: p.span(lo), Elements: elems}
}

func (p *parser) parseCompilationOperand() ast.Expr {
	lo := p.pos()
	switch {
	case p.atOper("!"):
		op := p.oper()
		x := p.parseCompilationOperand()
		return &ast.PrefixExpr{Span: p.span(lo), Op: op, X: x}

	case p.at(token.LPAREN):
		lp := p.pos()
		p.next()
		x := p.parseCompilationCond()
		rp := p.expect(token.RPAREN)
		return &ast.ParenExpr{Span: p.span(lo), Lparen: lp, X: x, Rparen: rp}

	case p.at(token.TRUE), p.at(token.FALSE):
		k := p.kind()
		p.next()
		return &ast.BasicLit{Span: p.span(lo), Kind: k}

	case p.at(token.IDENT):
		if isPlatformCondition(p.text(p.tok())) && p.peek(1) == token.LPAREN {
			return p.parsePlatformCond()
		}
		name := p.ident()
		return &ast.IdentExpr{Span: p.span(lo), Name: name}
	}
	p.errHere("expected a compilation condition")
	if !p.at(token.EOF) && !p.nl() {
		p.next()
	}
	return &ast.BadExpr{Span: p.span(lo)}
}

// isPlatformCondition lists the conditions the compiler answers
// itself. Everything else in a #if is a build flag, which is a bare
// name and takes no argument.
func isPlatformCondition(name string) bool {
	switch name {
	case "os", "arch", "swift", "compiler", "canImport",
		"targetEnvironment", "hasAttribute", "hasFeature",
		"_runtime", "_endian", "_pointerBitWidth", "_hasAtomicBitWidth",
		"_ptrauth", "_compiler_version":
		return true
	}
	return false
}

// parsePlatformCond reads one PlatformCondition. Its argument is not
// an expression — a version comparison has nothing on its left, and a
// module name denotes nothing in this program — so each form is read
// for what it is.
func (p *parser) parsePlatformCond() ast.Expr {
	lo := p.pos()
	c := &ast.PlatformCond{Name: p.ident()}
	name := p.f.Slice(c.Name.Lo, c.Name.Hi)
	c.Lparen = p.expect(token.LPAREN)

	switch string(name) {
	case "swift", "compiler":
		if p.atAnyOper() && (p.cur() == ">=" || p.cur() == "<") {
			c.Op, c.OpKind = p.pos(), p.kind()
			p.next()
		} else {
			p.errHere("expected '>=' or '<' in a version condition")
		}
		if p.at(token.INT_LIT) || p.at(token.FLOAT_LIT) {
			c.Ver = p.parseVersion()
		} else {
			p.errHere("expected a version number")
		}

	case "canImport":
		for {
			id := p.expectIdent()
			if id == nil {
				break
			}
			c.Path = append(c.Path, id)
			if !p.at(token.PERIOD) {
				break
			}
			p.next()
		}
		// `canImport(M, _version: 1.2.3)` asks about the module's
		// version as well as its presence. _underlyingVersion asks
		// about the Clang module behind it.
		if p.at(token.COMMA) {
			p.next()
			c.VerLabel = p.expectIdent()
			p.expect(token.COLON)
			p.parseCondVersion(c)
		}

	case "_compiler_version":
		p.parseCondVersion(c)

	default:
		if p.at(token.IDENT) || p.kind().IsKeyword() {
			c.Arg = p.ident()
		} else {
			p.errHere("expected a name in a platform condition")
		}
	}

	c.Rparen = p.expect(token.RPAREN)
	c.Span = p.span(lo)
	return c
}

func (p *parser) parseSourceLocation() ast.Stmt {
	lo := p.pos()
	s := &ast.SourceLocationStmt{Pound: p.pos()}
	p.next()
	s.Lparen = p.expect(token.LPAREN)
	if !p.at(token.RPAREN) {
		p.expectWord("file")
		p.expect(token.COLON)
		if x := p.parseStringLit(); x != nil {
			s.File = x
		}
		p.expect(token.COMMA)
		p.expectWord("line")
		p.expect(token.COLON)
		if p.at(token.INT_LIT) {
			llo := p.pos()
			p.next()
			s.Line = &ast.BasicLit{Span: p.span(llo), Kind: token.INT_LIT}
		} else {
			p.errHere("expected a line number")
		}
	}
	s.Rparen = p.expect(token.RPAREN)
	s.Span = p.span(lo)
	return s
}

func (p *parser) parseDiagnostic() ast.Stmt {
	lo := p.pos()
	s := &ast.DiagnosticStmt{Pound: p.pos(), Kind: p.kind()}
	p.next()
	s.Lparen = p.expect(token.LPAREN)
	if x := p.parseStringLit(); x != nil {
		s.Message = x
	} else {
		p.errHere("expected a string literal")
	}
	s.Rparen = p.expect(token.RPAREN)
	s.Span = p.span(lo)
	return s
}

// ---- code blocks ----

// parseCodeBlock reads '{' [Statements] '}'. Under SkipBodies the
// braces are matched and the contents dropped: a structural pass
// wants the declarations, not what they do.
func (p *parser) parseCodeBlock() *ast.CodeBlock {
	lo := p.pos()
	if !p.at(token.LBRACE) {
		p.errHere("expected '{'")
		return &ast.CodeBlock{Span: p.span(lo), Lbrace: token.NoPos, Rbrace: token.NoPos}
	}
	lbrace := p.pos()
	if p.mode&SkipBodies != 0 {
		p.skipBalanced()
		return &ast.CodeBlock{Span: p.span(lo), Lbrace: lbrace, Rbrace: p.prevEnd() - 1}
	}
	p.next()
	stmts := p.parseStmtList(stopBrace)
	rbrace := p.expect(token.RBRACE)
	return &ast.CodeBlock{Span: p.span(lo), Lbrace: lbrace, Stmts: stmts, Rbrace: rbrace}
}
