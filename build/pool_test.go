package build_test

import (
	"os/exec"
	"strings"
	"testing"
)

// The worker pool: a detached task runs on a worker and is joined from
// the main executor; @MainActor code awaited from it hops to the main
// thread and back; the main task's own Task { } stays on the main
// thread. The same program answers the same with no pool, one worker
// and several, since where a task runs is never something it can tell
// except through the checks written into it -- and a check that fails
// ends the program, as MainActor.assumeIsolated does.
func TestPoolHopsAndJoins(t *testing.T) {
	bin := buildSwift(t, "main", `
@MainActor func here() { MainActor.assumeIsolated { } }

@MainActor
final class Counter {
    var n = 0
    func bump() { here(); n += 1 }
    func read() async -> Int { here(); await Task.yield(); here(); return n }
}

func work(_ c: Counter, _ k: Int) async -> Int {
    var total = 0
    var i = 0
    while i < k {
        await c.bump()
        total += await c.read()
        await Task.yield()
        i += 1
    }
    await MainActor.run { here(); c.bump() }
    return total
}

@MainActor func main() async -> Int32 {
    let c = Counter()
    let a = Task.detached { () -> Int in return await work(c, 3) }
    let b = Task.detached { () -> Int in return await work(c, 2) }
    let inner = Task { () -> Int in here(); c.bump(); return c.n }
    let ra = await a.value
    let rb = await b.value
    let ri = await inner.value
    here()
    // Each work bumps k times and once more in MainActor.run; inner once.
    print("count", c.n, "sum", ra + rb, "inner", ri > 0)
    return 0
}
`, "")
	for _, workers := range []string{"0", "1", "3"} {
		cmd := exec.Command(bin)
		cmd.Env = append(cmd.Environ(), "VERTEX_WORKERS="+workers)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("VERTEX_WORKERS=%s: %v\n%s", workers, err, out)
		}
		got := strings.TrimSpace(string(out))
		// The sums depend on the interleaving of the two workers, so
		// only the count -- 3 + 1 + 2 + 1 + 1 -- and the shape are held.
		if !strings.HasPrefix(got, "count 8 sum ") || !strings.HasSuffix(got, " inner true") {
			t.Errorf("VERTEX_WORKERS=%s: got %q", workers, got)
		}
	}
}

// assumeIsolated outside the main thread ends the program, in Swift's
// words. With no pool every task is on the main thread, so the same
// program carries on.
func TestAssumeIsolatedOffMainThreadTraps(t *testing.T) {
	bin := buildSwift(t, "main", `
func main() async -> Int32 {
    let t = Task.detached {
        MainActor.assumeIsolated { }
        print("assumed")
    }
    await t.value
    return 0
}
`, "")
	run := func(workers string) (string, error) {
		cmd := exec.Command(bin)
		cmd.Env = append(cmd.Environ(), "VERTEX_WORKERS="+workers)
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	if out, err := run("0"); err != nil || strings.TrimSpace(out) != "assumed" {
		t.Errorf("with no pool: %v\n%s", err, out)
	}
	out, err := run("2")
	if err == nil {
		t.Fatalf("assumeIsolated on a worker carried on:\n%s", out)
	}
	if !strings.Contains(out, "Incorrect actor executor assumption; Expected MainActor executor.") {
		t.Errorf("trapped without Swift's words:\n%s", out)
	}
}
