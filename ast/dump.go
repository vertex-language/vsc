package ast

import (
	"fmt"
	"io"
	"reflect"

	"github.com/vertex-language/vsc/token"
)

// Fdump prints the tree rooted at n, one node per line, indented by
// depth. Identifiers and literals appear with their text, and every
// node with its position as line:column.
func Fdump(w io.Writer, f *token.File, n Node) error {
	d := &dumper{w: w, f: f}
	d.dump(n, 0)
	return d.err
}

type dumper struct {
	w   io.Writer
	f   *token.File
	err error
}

func (d *dumper) printf(format string, args ...any) {
	if d.err == nil {
		_, d.err = fmt.Fprintf(d.w, format, args...)
	}
}

func (d *dumper) dump(n Node, depth int) {
	if d.err != nil || n == nil || isNil(n) {
		return
	}
	for i := 0; i < depth; i++ {
		d.printf("  ")
	}
	p := d.f.Position(n.Pos())
	d.printf("%s %d:%d", nodeName(n), p.Line, p.Column)

	switch n := n.(type) {
	case *Ident:
		d.printf(" %s", n.Name(d.f))
	case *BasicLit:
		d.printf(" %s %s", n.Kind, d.f.Slice(n.Lo, n.Hi))
	case *MagicLit:
		d.printf(" %s", n.Kind)
	case *StringText, *VersionLit:
		d.printf(" %s", d.f.Slice(n.Pos(), n.End()))
	case *OperatorExpr:
		d.printf(" %s", d.f.Slice(n.Lo, n.Hi))
	case *StringLit:
		if n.Multiline {
			d.printf(" multiline")
		}
		if n.Pounds > 0 {
			d.printf(" pounds=%d", n.Pounds)
		}
	case *VarDecl:
		d.printf(" %s", n.Kind)
	case *CaseClause:
		d.printf(" %s", n.Kind)
	case *ValueBindingPattern:
		d.printf(" %s", n.Kind)
	case *OptionalBinding:
		d.printf(" %s", n.Kind)
	case *CastExpr:
		d.printf(" %s", n.Kind)
	case *IfConfigClause:
		d.printf(" %s", n.Kind)
	case *ImportDecl:
		if n.Kind != token.ILLEGAL {
			d.printf(" %s", n.Kind)
		}
	case *Modifier:
		d.printf(" %s", d.f.Slice(n.Lo, n.Hi))
	}
	d.printf("\n")

	for _, c := range children(n) {
		d.dump(c, depth+1)
	}
}

func nodeName(n Node) string {
	t := reflect.TypeOf(n)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t.Name()
}
