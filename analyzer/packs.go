package analyzer

import (
	"strconv"
	"strings"

	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/types"
)

// Parameter packs (SE-0393, SE-0408).
//
// A function with a pack -- `func f<each T>(_ v: repeat each T)` -- is
// never checked or lowered as written. Each call says how long its packs
// are, and is a call of an expansion of the function for those lengths:
// a copy of the declaration with `each T` made T$0, T$1, ..., the pack
// parameter made one parameter per element, and every `repeat` in it
// unrolled -- a tuple's elements, a call's arguments, a `for ... in
// repeat each v` loop, one copy of its body per element. The expansion
// is an ordinary generic function, declared beside the original, checked,
// and lowered like any other.

// packParams are the names of a declaration's parameter packs, or nil.
func (c *checker) packParams(d *ast.FuncDecl) []string {
	if d == nil || d.Generics == nil {
		return nil
	}
	var out []string
	for _, p := range d.Generics.Params {
		if p != nil && p.Each.IsValid() && p.Name != nil {
			out = append(out, p.Name.Text(c.file))
		}
	}
	return out
}

// packDeclOf is the declaration of a function with parameter packs a call
// names, or nil.
func (c *checker) packDeclOf(e *ast.CallExpr, scope *Scope) *ast.FuncDecl {
	id, ok := e.Fun.(*ast.IdentExpr)
	if !ok || id.Name == nil || id.Args != nil {
		return nil
	}
	fs, ok := c.lookupValue(scope, id.Name.Text(c.file)).(*FuncSymbol)
	if !ok {
		return nil
	}
	for _, o := range fs.Overloads() {
		if d, ok := o.Decl().(*ast.FuncDecl); ok && len(c.packParams(d)) > 0 {
			return d
		}
	}
	return nil
}

// packCall checks a call of a function with parameter packs as a call of
// its expansion for the lengths the arguments give.
func (c *checker) packCall(e *ast.CallExpr, d *ast.FuncDecl, expected types.Type, scope *Scope) (types.Type, bool) {
	lengths, ok := c.packLengths(e, d)
	if !ok {
		return nil, false
	}
	name := c.packExpansion(d, lengths, scope)
	if name == "" {
		return nil, false
	}
	at := ast.Span{Lo: e.Fun.Pos(), Hi: e.Fun.End()}
	call := &ast.CallExpr{Span: e.Span, Args: e.Args, Trailing: e.Trailing,
		Fun: &ast.IdentExpr{Span: at, Name: &ast.Ident{Span: at, Synth: name}}}
	t := c.checkExpr(call, expected, scope)
	c.info.ImplicitSelf[e] = call
	return t, true
}

// packLengths is how many arguments each pack of d takes at this call: a
// pack parameter takes its labelled argument and every unlabelled one
// after it, up to the next labelled one.
func (c *checker) packLengths(e *ast.CallExpr, d *ast.FuncDecl) (map[string]int, bool) {
	var args []*ast.CallArg
	if e.Args != nil {
		args = e.Args.Args
	}
	packs := map[string]bool{}
	for _, n := range c.packParams(d) {
		packs[n] = true
	}
	lengths := map[string]int{}
	ai := 0
	for _, p := range d.Sig.Params {
		label := ""
		if p.Label != nil {
			label = p.Label.Text(c.file)
		} else if p.Name != nil {
			label = p.Name.Text(c.file)
		}
		if label == "_" {
			label = ""
		}
		labelOf := func(a *ast.CallArg) string {
			if a.Label == nil {
				return ""
			}
			return a.Label.Text(c.file)
		}
		exp, isPack := p.Type.(*ast.PackExpansionType)
		if !isPack {
			if ai < len(args) && labelOf(args[ai]) == label {
				ai++
			}
			continue
		}
		n := 0
		if ai < len(args) && labelOf(args[ai]) == label {
			n = 1
			for ai+n < len(args) && args[ai+n].Label == nil {
				n++
			}
		}
		ai += n
		for _, name := range c.packRefs(exp.Base) {
			if !packs[name] {
				continue
			}
			if have, seen := lengths[name]; seen && have != n {
				return nil, false
			}
			lengths[name] = n
		}
	}
	if ai != len(args) {
		return nil, false
	}
	for n := range packs {
		if _, ok := lengths[n]; !ok {
			lengths[n] = 0
		}
	}
	return lengths, true
}

// packRefs are the names `each X` refers to inside a type.
func (c *checker) packRefs(t ast.Node) []string {
	var out []string
	ast.Inspect(t, func(n ast.Node) bool {
		if r, ok := n.(*ast.PackReferenceType); ok {
			if id, ok := r.Base.(*ast.IdentType); ok && id.Name != nil {
				out = append(out, id.Name.Text(c.file))
			}
		}
		return true
	})
	return out
}

// packExpansion is the name of d's expansion for these lengths, declared
// and checked the first time it is asked for.
func (c *checker) packExpansion(d *ast.FuncDecl, lengths map[string]int, scope *Scope) string {
	base := d.Name.Text(c.file)
	names := c.packParams(d)
	var key strings.Builder
	key.WriteString(base)
	for _, n := range names {
		key.WriteString("$")
		key.WriteString(strconv.Itoa(lengths[n]))
	}
	name := key.String()
	if c.packExpansions == nil {
		c.packExpansions = map[*ast.FuncDecl]map[string]bool{}
	}
	if c.packExpansions[d] == nil {
		c.packExpansions[d] = map[string]bool{}
	}
	if c.packExpansions[d][name] {
		return name
	}
	c.packExpansions[d][name] = true

	x := &packExpander{c: c, lengths: lengths, values: map[string]int{}, idx: -1}
	for _, p := range d.Sig.Params {
		exp, ok := p.Type.(*ast.PackExpansionType)
		if !ok {
			continue
		}
		n := -1
		for _, ref := range c.packRefs(exp.Base) {
			if l, ok := lengths[ref]; ok {
				n = l
			}
		}
		if n < 0 {
			continue
		}
		if p.Name != nil {
			x.values[p.Name.Text(c.file)] = n
		} else if p.Label != nil {
			x.values[p.Label.Text(c.file)] = n
		}
	}
	out := x.decl(d, name)
	stmt := &ast.DeclStmt{Span: d.Span, D: out}

	// Declared where the original is, and checked there.
	declScope := scope
	for s := scope; s != nil; s = s.parent {
		if s.elems[base] != nil && !s.members {
			declScope = s
		}
	}
	c.declareFunctions([]ast.Decl{out}, declScope)
	c.checkStmt(stmt, declScope)
	// And lowered with the file's own declarations.
	for _, f := range c.files {
		if f.Unit == c.file {
			f.Stmts = append(f.Stmts, stmt)
			break
		}
	}
	return name
}

// A packExpander copies a declaration for pack lengths. idx is the
// element of the packs the part being copied is for, inside a `repeat`,
// and -1 outside one.
type packExpander struct {
	c       *checker
	lengths map[string]int // a type pack's length, by name
	values  map[string]int // a value pack's length, by name
	idx     int
}

func elementName(name string, i int) string { return name + "$" + strconv.Itoa(i) }

// decl is d expanded, named name.
func (x *packExpander) decl(d *ast.FuncDecl, name string) *ast.FuncDecl {
	out := *d
	out.Name = &ast.Ident{Span: d.Name.Span, Synth: name}
	// Its generic parameters: a pack's become one per element.
	if d.Generics != nil {
		g := *d.Generics
		g.Params = nil
		for _, p := range d.Generics.Params {
			if p == nil || p.Name == nil {
				continue
			}
			n, isPack := x.lengths[p.Name.Text(x.c.file)]
			if !p.Each.IsValid() || !isPack {
				g.Params = append(g.Params, x.clone(p).(*ast.GenericParam))
				continue
			}
			for i := 0; i < n; i++ {
				q := *p
				q.Each = 0
				q.Name = &ast.Ident{Span: p.Name.Span, Synth: elementName(p.Name.Text(x.c.file), i)}
				if p.Inherit != nil {
					q.Inherit = x.clone(p.Inherit).(*ast.InheritanceClause)
				}
				g.Params = append(g.Params, &q)
			}
		}
		out.Generics = &g
		if len(g.Params) == 0 {
			out.Generics = nil
		}
	}
	// Its parameters: a pack's become one per element, the first with
	// the pack's label.
	sig := *d.Sig
	sig.Params = nil
	for _, p := range d.Sig.Params {
		exp, isPack := p.Type.(*ast.PackExpansionType)
		if !isPack {
			sig.Params = append(sig.Params, x.clone(p).(*ast.Param))
			continue
		}
		pname := ""
		if p.Name != nil {
			pname = p.Name.Text(x.c.file)
		} else if p.Label != nil {
			pname = p.Label.Text(x.c.file)
		}
		n := x.values[pname]
		for i := 0; i < n; i++ {
			q := *p
			q.Label = p.Label
			if p.Name == nil && p.Label != nil {
				q.Label = p.Label
			}
			if i > 0 {
				q.Label = &ast.Ident{Span: p.Span, Synth: "_"}
			}
			q.Name = &ast.Ident{Span: p.Span, Synth: elementName(pname, i)}
			q.Type = x.at(i, exp.Base).(ast.Type)
			sig.Params = append(sig.Params, &q)
		}
	}
	if d.Sig.Result != nil {
		r := *d.Sig.Result
		r.Type = x.clone(d.Sig.Result.Type).(ast.Type)
		sig.Result = &r
	}
	out.Sig = &sig
	if d.Where != nil {
		out.Where = x.clone(d.Where).(*ast.GenericWhereClause)
	}
	if d.Body != nil {
		out.Body = x.clone(d.Body).(*ast.CodeBlock)
	}
	return &out
}

// at copies n for one element of the packs.
func (x *packExpander) at(i int, n ast.Node) ast.Node {
	prev := x.idx
	x.idx = i
	defer func() { x.idx = prev }()
	return x.clone(n)
}

// clone copies n, expanding what refers to the packs.
func (x *packExpander) clone(n ast.Node) ast.Node {
	return ast.Clone(n, x.replace)
}

// packLength is how many elements a `repeat` pattern repeats over: the
// length of a pack it refers to.
func (x *packExpander) packLength(n ast.Node) int {
	length := -1
	ast.Inspect(n, func(m ast.Node) bool {
		switch r := m.(type) {
		case *ast.PackReferenceType:
			if id, ok := r.Base.(*ast.IdentType); ok && id.Name != nil {
				if l, ok := x.lengths[id.Name.Text(x.c.file)]; ok {
					length = l
				}
			}
		case *ast.PackReferenceExpr:
			if id, ok := r.X.(*ast.IdentExpr); ok && id.Name != nil {
				if l, ok := x.values[id.Name.Text(x.c.file)]; ok {
					length = l
				}
			}
		}
		return true
	})
	if length < 0 {
		return 0
	}
	return length
}

// replace is what stands in for a node: an element of a pack for `each`,
// and a `repeat` unrolled where one is written.
func (x *packExpander) replace(n ast.Node) ast.Node {
	switch v := n.(type) {
	case *ast.PackReferenceType:
		if id, ok := v.Base.(*ast.IdentType); ok && id.Name != nil && x.idx >= 0 {
			if _, isPack := x.lengths[id.Name.Text(x.c.file)]; isPack {
				return &ast.IdentType{Span: id.Span, Name: &ast.Ident{Span: id.Name.Span,
					Synth: elementName(id.Name.Text(x.c.file), x.idx)}}
			}
		}
	case *ast.PackReferenceExpr:
		if id, ok := v.X.(*ast.IdentExpr); ok && id.Name != nil && x.idx >= 0 {
			if _, isPack := x.values[id.Name.Text(x.c.file)]; isPack {
				return &ast.IdentExpr{Span: id.Span, Name: &ast.Ident{Span: id.Name.Span,
					Synth: elementName(id.Name.Text(x.c.file), x.idx)}}
			}
		}
	case *ast.ParenType:
		// `(repeat X)`: a tuple of X for each element, or the one X.
		exp, ok := v.X.(*ast.PackExpansionType)
		if !ok {
			return nil
		}
		n := x.packLength(exp.Base)
		if n == 1 {
			return x.at(0, exp.Base)
		}
		out := &ast.TupleType{Span: v.Span, Lparen: v.Lparen, Rparen: v.Rparen}
		for i := 0; i < n; i++ {
			out.Elems = append(out.Elems, &ast.TupleTypeElem{Span: v.Span, Type: x.at(i, exp.Base).(ast.Type)})
		}
		return out
	case *ast.ParenExpr:
		exp, ok := v.X.(*ast.PackExpansionExpr)
		if !ok {
			return nil
		}
		n := x.packLength(exp.X)
		if n == 1 {
			return &ast.ParenExpr{Span: v.Span, X: x.at(0, exp.X).(ast.Expr)}
		}
		out := &ast.TupleExpr{Span: v.Span, Lparen: v.Pos(), Rparen: v.End()}
		for i := 0; i < n; i++ {
			out.Elems = append(out.Elems, &ast.TupleElem{Span: v.Span, X: x.at(i, exp.X).(ast.Expr)})
		}
		return out
	case *ast.TupleType:
		if !hasPackElement(v) {
			return nil
		}
		out := *v
		out.Elems = nil
		for _, el := range v.Elems {
			if exp, ok := el.Type.(*ast.PackExpansionType); ok {
				for i := 0; i < x.packLength(exp.Base); i++ {
					out.Elems = append(out.Elems, &ast.TupleTypeElem{Span: el.Span, Type: x.at(i, exp.Base).(ast.Type)})
				}
				continue
			}
			out.Elems = append(out.Elems, x.clone(el).(*ast.TupleTypeElem))
		}
		return &out
	case *ast.TupleExpr:
		if !hasPackExpr(v) {
			return nil
		}
		out := *v
		out.Elems = nil
		for _, el := range v.Elems {
			if exp, ok := el.X.(*ast.PackExpansionExpr); ok {
				for i := 0; i < x.packLength(exp.X); i++ {
					out.Elems = append(out.Elems, &ast.TupleElem{Span: el.Span, X: x.at(i, exp.X).(ast.Expr)})
				}
				continue
			}
			out.Elems = append(out.Elems, x.clone(el).(*ast.TupleElem))
		}
		// One element is that element, as `(repeat each v)` of one is.
		if len(out.Elems) == 1 && out.Elems[0].Label == nil {
			return &ast.ParenExpr{Span: v.Span, X: out.Elems[0].X}
		}
		return &out
	case *ast.CallArgs:
		expands := false
		for _, a := range v.Args {
			if _, ok := a.X.(*ast.PackExpansionExpr); ok {
				expands = true
			}
		}
		if !expands {
			return nil
		}
		out := *v
		out.Args = nil
		for _, a := range v.Args {
			if exp, ok := a.X.(*ast.PackExpansionExpr); ok {
				for i := 0; i < x.packLength(exp.X); i++ {
					arg := &ast.CallArg{Span: a.Span, X: x.at(i, exp.X).(ast.Expr)}
					if i == 0 {
						arg.Label = a.Label
					}
					out.Args = append(out.Args, arg)
				}
				continue
			}
			out.Args = append(out.Args, x.clone(a).(*ast.CallArg))
		}
		return &out
	case *ast.CodeBlock:
		loops := false
		for _, s := range v.Stmts {
			if f, ok := s.(*ast.ForInStmt); ok {
				if _, ok := f.Seq.(*ast.PackExpansionExpr); ok {
					loops = true
				}
			}
		}
		if !loops {
			return nil
		}
		out := *v
		out.Stmts = nil
		for _, s := range v.Stmts {
			f, ok := s.(*ast.ForInStmt)
			exp, isPack := ast.Expr(nil), false
			if ok {
				var e *ast.PackExpansionExpr
				e, isPack = f.Seq.(*ast.PackExpansionExpr)
				if isPack {
					exp = e.X
				}
			}
			if !isPack {
				out.Stmts = append(out.Stmts, x.clone(s).(ast.Stmt))
				continue
			}
			// `for v in repeat each xs { body }`: the body once for each
			// element, in a scope of its own that binds it.
			for i := 0; i < x.packLength(exp); i++ {
				sp := ast.Span{Lo: f.Pos(), Hi: f.End()}
				bind := letPattern(x.at(i, f.Pat).(ast.Pattern), x.at(i, exp).(ast.Expr), sp)
				body := x.at(i, f.Body).(*ast.CodeBlock)
				stmts := append([]ast.Stmt{bind}, body.Stmts...)
				out.Stmts = append(out.Stmts, &ast.DoStmt{Span: sp, Do: f.Pos(),
					Body: &ast.CodeBlock{Span: body.Span, Lbrace: body.Lbrace, Rbrace: body.Rbrace, Stmts: stmts}})
			}
		}
		return &out
	}
	return nil
}

func hasPackElement(t *ast.TupleType) bool {
	for _, el := range t.Elems {
		if _, ok := el.Type.(*ast.PackExpansionType); ok {
			return true
		}
	}
	return false
}

func hasPackExpr(t *ast.TupleExpr) bool {
	for _, el := range t.Elems {
		if _, ok := el.X.(*ast.PackExpansionExpr); ok {
			return true
		}
	}
	return false
}
