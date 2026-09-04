package parser

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/vertex-language/vsc/token"
)

// TestCorpus parses every file named by VSC_GLOB and reports the most
// common diagnostics.
func TestCorpus(t *testing.T) {
	files, err := filepath.Glob(os.Getenv("VSC_GLOB"))
	if err != nil || len(files) == 0 {
		t.Skip("no corpus")
	}
	counts := map[string]int{}
	examples := map[string]string{}
	nbad, total := 0, 0
	for _, name := range files {
		src, err := os.ReadFile(name)
		if err != nil {
			continue
		}
		total++
		f := token.NewFile(name, src)
		_, diags := ParseFile(f, 0)
		bad := false
		for _, d := range diags {
			if d.Severity != token.Error {
				continue
			}
			bad = true
			counts[d.Message]++
			if _, ok := examples[d.Message]; !ok {
				p := f.Position(d.Pos)
				examples[d.Message] = p.String() + ": " + string(f.LineText(p.Line))
			}
		}
		if bad {
			nbad++
		}
	}
	type kv struct {
		msg string
		n   int
	}
	var all []kv
	for m, n := range counts {
		all = append(all, kv{m, n})
	}
	sort.Slice(all, func(i, j int) bool { return all[i].n > all[j].n })
	t.Logf("%d/%d files with errors", nbad, total)
	for i, e := range all {
		if i > 25 {
			break
		}
		t.Logf("%5d  %s\n         %s", e.n, e.msg, strings.TrimSpace(examples[e.msg]))
	}
}
