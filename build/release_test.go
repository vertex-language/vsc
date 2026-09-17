package build_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"

	vsc "github.com/vertex-language/vsc"
	"github.com/vertex-language/vsc/build"
)

// TestObjectsReleaseWhatTheyOwn: when the last reference to an object
// goes, what its stored properties hold goes with it. A class that
// leaked its String and its Array would hold hundreds of megabytes by the
// end of this loop; one that releases them stays within a few.
func TestObjectsReleaseWhatTheyOwn(t *testing.T) {
	if _, ok := build.Host(); !ok || runtime.GOOS != "darwin" {
		t.Skip("peak memory is read the way macOS reports it")
	}
	const program = `
struct Wide { var first: String; var second: String; var n: Int }

class Holder {
    var name: String
    var tags: [Int32]
    init(_ n: String) { name = n + " and a suffix that makes it longer"; tags = [1, 2, 3, 4, 5, 6, 7, 8] }
}

func main() -> Int32 {
    var total = 0
    var i = 0
    while i < 2000000 {
        let h = Holder("a name long enough to be stored on the heap")
        total += h.tags.count
        i += 1
    }
    // Described, which copies each into the runtime's hands, and boxed
    // in an existential, which is wider than its buffer.
    var j = 0
    while j < 300000 {
        let w = Wide(first: "a heap string" + " and its suffix", second: "short", n: j)
        let s = "\(w)"
        print(w, s.count, terminator: "")
        j += 1
    }
    return total == 16000000 ? 42 : 1
}
`
	if peak := peakResident(t, program); peak > 32<<20 {
		t.Errorf("peak resident set %d MB: the objects' Strings and Arrays were not released", peak>>20)
	}
}

// TestCollectionVariablesReleaseWhatTheyHold: a var holding an Array, a
// Dictionary or a Set lets go of it where its scope ends, and growing
// storage returns the storage it grew out of. Leaking either holds
// hundreds of megabytes by the end of this loop.
func TestCollectionVariablesReleaseWhatTheyHold(t *testing.T) {
	if _, ok := build.Host(); !ok || runtime.GOOS != "darwin" {
		t.Skip("peak memory is read the way macOS reports it")
	}
	const program = `
func main() -> Int32 {
    var total = 0
    var round = 0
    while round < 3000 {
        var names: [String] = []
        var index: [String: Int] = [:]
        var seen: Set<String> = []
        var i = 0
        while i < 200 {
            let name = "a name long enough for the heap \(i)"
            names.append(name)
            index[name] = i
            seen.insert(name)
            i += 1
        }
        let snapshot = names
        names[0] = "changed"
        i = 0
        while i < 200 {
            if let at = index[snapshot[i]] { total += at }
            if i % 2 == 0 { index[snapshot[i]] = nil }
            i += 1
        }
        total += index.count + seen.count + names.count
        _ = names.removeLast()
        names.removeAll()
        round += 1
    }
    return total == 3000 * (19900 + 100 + 200 + 200) ? 42 : 1
}
`
	if peak := peakResident(t, program); peak > 32<<20 {
		t.Errorf("peak resident set %d MB: collections were not released", peak>>20)
	}
}

// peakResident builds program, runs it, requires it to exit 42, and
// answers its peak resident set in bytes.
func peakResident(t *testing.T, program string) int64 {
	t.Helper()
	target, _ := build.Host()
	unit, diags := vsc.Compile([]vsc.Source{{Name: "main.swift", Text: []byte(program)}},
		vsc.Options{Module: "main", Target: target})
	if len(diags) > 0 {
		t.Fatalf("refused: %v", diags)
	}
	obj, err := build.Object(unit.VIR, build.Options{})
	if err != nil {
		t.Fatal(err)
	}
	exe, err := build.Executable([]build.Input{{Name: "main.o", Data: obj}}, build.LinkOptions{Target: target})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "program")
	if err := os.WriteFile(path, exe, 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(path)
	_ = cmd.Run()
	if code := cmd.ProcessState.ExitCode(); code != 42 {
		t.Fatalf("exit status %d, want 42", code)
	}
	usage, ok := cmd.ProcessState.SysUsage().(*syscall.Rusage)
	if !ok {
		t.Skip("no resource usage for the child")
	}
	// ru_maxrss is bytes on macOS.
	return usage.Maxrss
}
