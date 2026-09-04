package ast

import (
	"bytes"
	"testing"

	"github.com/vertex-language/vsc/token"
)

func TestNodeSpans(t *testing.T) {
	file := token.NewFile("node.vs", []byte("[5 of Int]"))

	sizeExpr := &BasicLit{
		Span: Span{Lo: file.Pos(1), Hi: file.Pos(2)},
		Kind: token.INT_LIT,
	}
	elemType := &IdentType{
		Span: Span{Lo: file.Pos(6), Hi: file.Pos(9)},
		Name: &Ident{Span: Span{Lo: file.Pos(6), Hi: file.Pos(9)}},
	}
	sizedArray := &SizedArrayType{
		Span:    Span{Lo: file.Pos(0), Hi: file.Pos(10)},
		Lsquare: file.Pos(0),
		Size:    sizeExpr,
		Of:      file.Pos(3),
		Elem:    elemType,
		Rsquare: file.Pos(9),
	}

	if sizedArray.Pos() != file.Pos(0) {
		t.Errorf("expected Pos %v, got %v", file.Pos(0), sizedArray.Pos())
	}
	if sizedArray.End() != file.Pos(10) {
		t.Errorf("expected End %v, got %v", file.Pos(10), sizedArray.End())
	}

	var visited []string
	Inspect(sizedArray, func(n Node) bool {
		if n != nil {
			visited = append(visited, nodeName(n))
		}
		return true
	})

	expected := []string{"SizedArrayType", "BasicLit", "IdentType", "Ident"}
	if len(visited) != len(expected) {
		t.Fatalf("expected %d nodes, got %d: %v", len(expected), len(visited), visited)
	}

	var buf bytes.Buffer
	if err := Fdump(&buf, file, sizedArray); err != nil {
		t.Fatalf("Fdump failed: %v", err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("SizedArrayType")) {
		t.Errorf("dump missing SizedArrayType: %s", buf.String())
	}
}

func TestIdentMethods(t *testing.T) {
	src := []byte("plain `escaped`")
	f := token.NewFile("id.vs", src)

	id1 := &Ident{Span: Span{Lo: f.Pos(0), Hi: f.Pos(5)}}
	if id1.Name(f) != "plain" || id1.Text(f) != "plain" {
		t.Errorf("id1: unexpected %q / %q", id1.Name(f), id1.Text(f))
	}

	id2 := &Ident{Span: Span{Lo: f.Pos(6), Hi: f.Pos(15)}, Escaped: true}
	if id2.Name(f) != "`escaped`" || id2.Text(f) != "escaped" {
		t.Errorf("id2: unexpected %q / %q", id2.Name(f), id2.Text(f))
	}

	var nilIdent *Ident
	if nilIdent.Name(f) != "" || nilIdent.Text(f) != "" {
		t.Errorf("nil ident name/text should be empty")
	}
}

type testReleaser struct {
	released bool
}

func (r *testReleaser) Release() {
	r.released = true
}

func TestFileReleaser(t *testing.T) {
	fileNode := &File{}
	// Safe with no releaser
	fileNode.Release()

	r := &testReleaser{}
	fileNode.SetReleaser(r)
	fileNode.Release()
	if !r.released {
		t.Errorf("expected releaser to be called")
	}
	// Safe to call twice
	fileNode.Release()
}

func TestInspectEarlyTermination(t *testing.T) {
	f := token.NewFile("inspect.vs", []byte("a + b"))
	tree := &SequenceExpr{
		Span: Span{Lo: f.Pos(0), Hi: f.Pos(5)},
		Elements: []Expr{
			&IdentExpr{Span: Span{Lo: f.Pos(0), Hi: f.Pos(1)}},
			&IdentExpr{Span: Span{Lo: f.Pos(4), Hi: f.Pos(5)}},
		},
	}

	count := 0
	Inspect(tree, func(n Node) bool {
		if n != nil {
			count++
		}
		return false // prune immediately
	})
	if count != 1 {
		t.Errorf("expected 1 node visited on early termination, got %d", count)
	}

	// Walk with nil
	Walk(inspector(func(n Node) bool { return true }), nil)
	var typedNil *IdentExpr
	Walk(inspector(func(n Node) bool { return true }), typedNil)
}

func TestDumpAllNodeKinds(t *testing.T) {
	src := []byte("line1\nline2\nline3\nline4")
	f := token.NewFile("dump.vs", src)

	nodes := []Node{
		&MagicLit{Span: Span{Lo: f.Pos(0), Hi: f.Pos(5)}, Kind: token.POUND_FILE},
		&StringText{Span: Span{Lo: f.Pos(0), Hi: f.Pos(5)}},
		&VersionLit{Span: Span{Lo: f.Pos(0), Hi: f.Pos(5)}},
		&OperatorExpr{Span: Span{Lo: f.Pos(0), Hi: f.Pos(5)}},
		&StringLit{Span: Span{Lo: f.Pos(0), Hi: f.Pos(5)}, Multiline: true, Pounds: 2},
		&VarDecl{Span: Span{Lo: f.Pos(0), Hi: f.Pos(5)}, Kind: token.LET},
		&CaseClause{Span: Span{Lo: f.Pos(0), Hi: f.Pos(5)}, Kind: token.CASE},
		&ValueBindingPattern{Span: Span{Lo: f.Pos(0), Hi: f.Pos(5)}, Kind: token.VAR},
		&OptionalBinding{Span: Span{Lo: f.Pos(0), Hi: f.Pos(5)}, Kind: token.LET},
		&CastExpr{Span: Span{Lo: f.Pos(0), Hi: f.Pos(5)}, Kind: token.AS},
		&IfConfigClause{Span: Span{Lo: f.Pos(0), Hi: f.Pos(5)}, Kind: token.POUND_IF},
		&ImportDecl{Span: Span{Lo: f.Pos(0), Hi: f.Pos(5)}, Kind: token.STRUCT},
		&Modifier{Span: Span{Lo: f.Pos(0), Hi: f.Pos(5)}},
	}

	for _, n := range nodes {
		var buf bytes.Buffer
		if err := Fdump(&buf, f, n); err != nil {
			t.Errorf("failed to dump %T: %v", n, err)
		}
		if buf.Len() == 0 {
			t.Errorf("empty dump for %T", n)
		}
	}
}
