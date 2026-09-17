package parser

import (
	"fmt"

	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/scanner"
	"github.com/vertex-language/vsc/token"
)

// Mode controls optional parser behavior.
type Mode uint

const (
	// ParseComments retains comment tokens on the File.
	ParseComments Mode = 1 << iota
	// SkipBodies skips function, initializer, and accessor bodies without parsing them.
	SkipBodies
	// Tolerant continues parsing past the normal resync budget (useful for IDEs).
	Tolerant
)

// DefaultMode is zero.
const DefaultMode Mode = 0

const (
	maxResync = 100  // recovery attempts before going dead
	maxDepth  = 1000 // nesting cap: expressions, types, patterns, statements
)

// ParseFile runs the scanner itself and parses the file. The tree is
// never nil; scanner and parser diagnostics arrive merged and sorted.
func ParseFile(f *token.File, mode Mode) (*ast.File, []token.Diagnostic) {
	var sm scanner.Mode
	if mode&ParseComments != 0 {
		sm = scanner.ScanComments
	}
	toks, diags := scanner.Scan(f, sm)

	p := &parser{f: f, mode: mode, diags: diags}
	file := &ast.File{Unit: f}
	file.SetReleaser(&arena{})

	if mode&ParseComments != 0 {
		for _, t := range toks {
			if t.Kind == token.COMMENT {
				file.Comments = append(file.Comments, t)
			} else {
				p.toks = append(p.toks, t)
			}
		}
	} else {
		p.toks = toks
	}

	file.Stmts = p.parseStmtList(stopEOF)
	if !p.at(token.EOF) {
		p.errHere("expected a declaration or a statement")
	}

	lo, hi := p.toks[0].Pos, p.toks[len(p.toks)-1].End
	if hi <= lo {
		hi = lo + 1
	}
	file.Span = ast.Span{Lo: lo, Hi: hi}
	token.SortDiagnostics(p.diags)
	return file, p.diags
}

// arena implements ast.Releaser for memory reclamation after parsing.
type arena struct{}

func (*arena) Release() {}

type parser struct {
	f    *token.File
	toks []token.Token
	i    int
	mode Mode

	// split is the byte offset consumed in multi-character operator tokens (e.g. `>>`).
	split int

	diags   []token.Diagnostic
	quiet   bool      // reported; no token consumed since
	lastErr token.Pos // never report twice at one position
	resyncs int
	dead    bool // past the budget, not Tolerant: run to EOF silently
	depth   int

	// inCoroutine indicates whether parsing within a _read or _modify accessor.
	inCoroutine bool
}

// ---- token access ----

func (p *parser) tok() token.Token { return p.toks[p.i] }
func (p *parser) kind() token.Kind { return p.toks[p.i].Kind }
func (p *parser) at(k token.Kind) bool {
	return p.toks[p.i].Kind == k && p.split == 0
}

// pos is the start of what is left of the current token.
func (p *parser) pos() token.Pos { return p.toks[p.i].Pos + token.Pos(p.split) }

func (p *parser) peekTok(n int) token.Token {
	if p.i+n >= len(p.toks) {
		return p.toks[len(p.toks)-1]
	}
	return p.toks[p.i+n]
}

func (p *parser) peek(n int) token.Kind { return p.peekTok(n).Kind }

func (p *parser) next() {
	if p.kind() != token.EOF {
		p.i++
		p.split = 0
		p.quiet = false
	}
}

func (p *parser) prevEnd() token.Pos {
	if p.split > 0 {
		return p.pos()
	}
	if p.i == 0 {
		return p.toks[0].Pos
	}
	return p.toks[p.i-1].End
}

// span returns an ast.Span from lo to the end of the previous token.
func (p *parser) span(lo token.Pos) ast.Span {
	hi := p.prevEnd()
	if hi <= lo {
		hi = lo + 1
	}
	return ast.Span{Lo: lo, Hi: hi}
}

// text is a token's spelling.
func (p *parser) text(t token.Token) string { return string(p.f.Slice(t.Pos, t.End)) }

// cur is what is left of the current token's spelling.
func (p *parser) cur() string {
	t := p.tok()
	return string(p.f.Slice(t.Pos+token.Pos(p.split), t.End))
}

// nl reports whether a line terminator precedes the current token —
// which is what ends a statement, a modifier run, or a trailing
// closure's chance to attach.
func (p *parser) nl() bool { return p.tok().Flags.Has(token.FlagNLBefore) }

// atWord reports whether the current token is the contextual keyword
// s. A backtick-escaped spelling never matches: `get` is a name.
func (p *parser) atWord(s string) bool {
	t := p.tok()
	return t.Kind == token.IDENT && p.split == 0 &&
		!t.Flags.Has(token.FlagEscaped) && p.text(t) == s
}

// wordAt is atWord n tokens ahead.
func (p *parser) wordAt(n int, s string) bool {
	t := p.peekTok(n)
	return t.Kind == token.IDENT && !t.Flags.Has(token.FlagEscaped) && p.text(t) == s
}

// more advances past a comma separator and reports whether another item follows.
func (p *parser) more(start int) bool {
	if !p.at(token.COMMA) {
		return false
	}
	p.next()
	if p.i == start {
		p.next()
	}
	return true
}

// takeWordBeforeType consumes contextual keyword s if immediately followed by a type.
func (p *parser) takeWordBeforeType(s string) token.Pos {
	if !p.atWord(s) || !p.atTypeStartAt(1) {
		return token.NoPos
	}
	pos := p.pos()
	p.next()
	return pos
}

// takeWord consumes the contextual keyword s if it is here, and
// returns its position.
func (p *parser) takeWord(s string) token.Pos {
	if !p.atWord(s) {
		return token.NoPos
	}
	pos := p.pos()
	p.next()
	return pos
}

// ident consumes the current token as an identifier, whatever it is.
func (p *parser) ident() *ast.Ident {
	t := p.tok()
	p.next()
	return &ast.Ident{
		Span:    ast.Span{Lo: t.Pos, Hi: t.End},
		Escaped: t.Flags.Has(token.FlagEscaped),
	}
}

// ---- operators ----

// atOper reports whether the current token is an operator of any
// position whose remaining spelling is s.
func (p *parser) atOper(s string) bool {
	return p.kind().IsOperator() && p.cur() == s
}

// atAnyOper reports whether an operator of any position is here.
func (p *parser) atAnyOper() bool { return p.kind().IsOperator() }

// oper consumes the current operator token and returns it as an
// expression: a reference to whatever the name denotes, unresolved.
func (p *parser) oper() *ast.OperatorExpr {
	t := p.tok()
	lo := p.pos()
	p.next()
	return &ast.OperatorExpr{Span: ast.Span{Lo: lo, Hi: t.End}, Kind: t.Kind}
}

// atOperHead reports whether what is left of the current token is an
// operator beginning with character c.
func (p *parser) atOperHead(c byte) bool {
	switch p.kind() {
	case token.OPER_PREFIX, token.OPER_BINARY, token.OPER_POSTFIX,
		token.QUESTION_POSTFIX, token.QUESTION_INFIX, token.EXCLAIM_POSTFIX,
		token.ASSIGN, token.ARROW, token.AMP_PREFIX:
		s := p.cur()
		return len(s) > 0 && s[0] == c
	}
	return false
}

// atLt reports whether a generic clause's '<' is here.
func (p *parser) atLt() bool { return p.atOperHead('<') }

// atGt is the same for a closing '>'.
func (p *parser) atGt() bool { return p.atOperHead('>') }

// glued reports whether the current token is immediately adjacent to the previous one.
func (p *parser) glued() bool {
	return p.split > 0 || p.tok().Flags.Has(token.FlagAdjacent)
}

// takeLt consumes one '<' from the front of the current operator.
func (p *parser) takeLt() token.Pos { return p.takeOperChar() }

// takeGt consumes one '>' from the front of the current operator,
// splitting the token if more of it remains.
func (p *parser) takeGt() token.Pos { return p.takeOperChar() }

// takeOperChar consumes one character of the current operator token,
// splitting it if more of it remains.
func (p *parser) takeOperChar() token.Pos {
	pos := p.pos()
	if len(p.cur()) > 1 {
		p.split++
		p.quiet = false
		return pos
	}
	p.next()
	return pos
}

// ---- diagnostics ----

// errHere reports an error diagnostic at the current token position.
func (p *parser) errHere(msg string) {
	if p.dead || p.quiet {
		return
	}
	t := p.tok()
	p.quiet = true
	pos := p.pos()
	if pos <= p.lastErr {
		return
	}
	p.lastErr = pos
	end := t.End
	if end <= pos {
		end = pos + 1 // EOF's zero-width span
	}
	p.diags = append(p.diags, token.Diagnostic{
		Pos: pos, End: end, Severity: token.Error, Message: msg,
	})
}

// errAt reports an error diagnostic at the given node's span.
func (p *parser) errAt(n ast.Node, msg string) {
	if p.dead {
		return
	}
	lo, hi := n.Pos(), n.End()
	if hi <= lo {
		hi = lo + 1
	}
	p.diags = append(p.diags, token.Diagnostic{
		Pos: lo, End: hi, Severity: token.Error, Message: msg,
	})
}

// warnHere reports a warning at the current token.
func (p *parser) warnHere(msg string) {
	if p.dead {
		return
	}
	t := p.tok()
	pos := p.pos()
	end := t.End
	if end <= pos {
		end = pos + 1
	}
	p.diags = append(p.diags, token.Diagnostic{
		Pos: pos, End: end, Severity: token.Warn, Message: msg,
	})
}

func (p *parser) expect(k token.Kind) token.Pos {
	if p.at(k) {
		pos := p.pos()
		p.next()
		return pos
	}
	p.errHere(fmt.Sprintf("expected '%s'", k))
	return token.NoPos
}

// expectWord is expect for a contextual keyword.
func (p *parser) expectWord(s string) token.Pos {
	if pos := p.takeWord(s); pos.IsValid() {
		return pos
	}
	p.errHere(fmt.Sprintf("expected '%s'", s))
	return token.NoPos
}

// expectIdent consumes and returns an identifier, reporting an error if missing or a keyword.
func (p *parser) expectIdent() *ast.Ident {
	switch {
	case p.at(token.IDENT):
		return p.ident()
	case p.kind().IsKeyword():
		p.errHere(fmt.Sprintf("'%s' is a keyword and cannot be used as a name; write it as `%s`",
			p.kind(), p.kind()))
		return p.ident()
	}
	p.errHere("expected an identifier")
	return nil
}

// ---- speculation ----

// mark records everything a speculative parse may change.
type mark struct {
	i, split, resyncs, ndiags int
	quiet                     bool
	lastErr                   token.Pos
	dead                      bool
}

func (p *parser) mark() mark {
	return mark{i: p.i, split: p.split, resyncs: p.resyncs, ndiags: len(p.diags),
		quiet: p.quiet, lastErr: p.lastErr, dead: p.dead}
}

// reset restores the parser state to m, discarding speculative diagnostics.
func (p *parser) reset(m mark) {
	p.i, p.split, p.resyncs = m.i, m.split, m.resyncs
	p.diags = p.diags[:m.ndiags]
	p.quiet, p.lastErr, p.dead = m.quiet, m.lastErr, m.dead
}

// ---- recovery ----

var (
	declFollow  = map[token.Kind]bool{token.SEMI: true, token.RBRACE: true}
	parenFollow = map[token.Kind]bool{token.RPAREN: true, token.SEMI: true}
	braceFollow = map[token.Kind]bool{token.COMMA: true, token.RBRACE: true}
	stmtFollow  = map[token.Kind]bool{token.SEMI: true, token.RBRACE: true}
)

// advanceTo resynchronizes to a follow set, stepping over balanced bracket groups.
func (p *parser) advanceTo(follow map[token.Kind]bool) {
	p.resyncs++
	if p.resyncs > maxResync && p.mode&Tolerant == 0 {
		p.dead = true
	}
	if p.dead {
		p.i = len(p.toks) - 1 // the EOF token
		p.split = 0
		return
	}
	for {
		k := p.kind()
		if k == token.EOF || (follow[k] && p.split == 0) {
			return
		}
		switch k {
		case token.LPAREN, token.LSQUARE, token.LBRACE:
			p.skipBalanced()
		default:
			p.next()
		}
	}
}

// skipBalanced consumes an opener through its matching closer.
func (p *parser) skipBalanced() {
	depth := 0
	for {
		switch p.kind() {
		case token.LPAREN, token.LSQUARE, token.LBRACE:
			depth++
		case token.RPAREN, token.RSQUARE, token.RBRACE:
			depth--
		case token.EOF:
			return
		}
		p.next()
		if depth <= 0 {
			return
		}
	}
}

// tooDeep is the maxDepth guard; on breach it reports once and
// consumes a token so callers returning Bad* still make progress.
func (p *parser) tooDeep() bool {
	if p.depth <= maxDepth {
		return false
	}
	p.errHere("expression, type or statement is too deeply nested")
	if !p.at(token.EOF) {
		p.next()
	}
	return true
}
