package build_test

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"
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

// A Task { } started on a thread that is no executor's -- an OS thread a
// C library or a package like vm makes -- goes to the main executor by
// its inbox, as one from another executor does. It used to be put
// straight on the main executor's run list, which only the main thread
// may touch, and under load tasks were lost or the list corrupted.
func TestTasksStartedOnForeignThreads(t *testing.T) {
	bin := buildSwift(t, "main", `
var ran = 0

@_cdecl("start_one")
func startOne() {
    Task { ran += 1 }
}

@_silgen_name("start_threads")
func startThreads(_ threads: Int32, _ each: Int32)

@_silgen_name("join_threads")
func joinThreads()

func main() async -> Int32 {
    let threads: Int32 = 4
    let each: Int32 = 5000
    startThreads(threads, each)
    var waited = 0
    while ran < Int(threads * each) && waited < 5000 {
        try? await Task.sleep(nanoseconds: 1_000_000)
        waited += 1
    }
    joinThreads()
    print("ran", ran)
    return 0
}
`, `
#include <pthread.h>
#include <stdint.h>

void start_one(void);

static pthread_t threads[16];
static int32_t count, each;

static void* body(void* arg) {
    (void)arg;
    for (int32_t i = 0; i < each; i++) start_one();
    return 0;
}

void start_threads(int32_t n, int32_t k) {
    count = n;
    each = k;
    for (int32_t i = 0; i < n; i++) pthread_create(&threads[i], 0, body, 0);
}

void join_threads(void) {
    for (int32_t i = 0; i < count; i++) pthread_join(threads[i], 0);
}
`)
	for _, workers := range []string{"0", "3"} {
		// A corrupted run list hangs the program rather than failing it.
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		cmd := exec.CommandContext(ctx, bin)
		cmd.Env = append(cmd.Environ(), "VERTEX_WORKERS="+workers)
		out, err := cmd.CombinedOutput()
		cancel()
		if err != nil {
			t.Fatalf("VERTEX_WORKERS=%s: %v\n%s", workers, err, out)
		}
		if got := strings.TrimSpace(string(out)); got != "ran 20000" {
			t.Errorf("VERTEX_WORKERS=%s: got %q, want every task run", workers, got)
		}
	}
}
