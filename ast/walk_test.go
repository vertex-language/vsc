package ast

import (
	"bytes"
	"testing"

	"github.com/vertex-language/vsc/token"
)

func TestWalkAndDump(t *testing.T) {
	file := token.NewFile("test.vs", []byte("repeat each x"))

	ref := &PackReferenceExpr{
		Span: Span{Lo: file.Pos(7), Hi: file.Pos(13)},
		Each: file.Pos(7),
		X: &IdentExpr{
			Span: Span{Lo: file.Pos(12), Hi: file.Pos(13)},
			Name: &Ident{Span: Span{Lo: file.Pos(12), Hi: file.Pos(13)}},
		},
	}
	exp := &PackExpansionExpr{
		Span:   Span{Lo: file.Pos(0), Hi: file.Pos(13)},
		Repeat: file.Pos(0),
		X:      ref,
	}

	var visited []string
	Inspect(exp, func(n Node) bool {
		if n != nil {
			visited = append(visited, nodeName(n))
		}
		return true
	})

	expected := []string{"PackExpansionExpr", "PackReferenceExpr", "IdentExpr", "Ident"}
	if len(visited) != len(expected) {
		t.Fatalf("expected %d nodes, got %d: %v", len(expected), len(visited), visited)
	}
	for i, expName := range expected {
		if visited[i] != expName {
			t.Errorf("at index %d: expected %s, got %s", i, expName, visited[i])
		}
	}

	var buf bytes.Buffer
	if err := Fdump(&buf, file, exp); err != nil {
		t.Fatalf("Fdump failed: %v", err)
	}
	dumpStr := buf.String()
	t.Logf("Dump:\n%s", dumpStr)
	if !bytes.Contains(buf.Bytes(), []byte("PackReferenceExpr")) {
		t.Errorf("expected dump to contain PackReferenceExpr")
	}
}
