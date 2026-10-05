// Package timing records where a build's time goes, phase by phase.
//
// With VSC_TIME=1 in the environment, every phase the compiler runs adds
// its duration here, and the command prints the totals when the build
// ends: how long each phase took summed over every module, and how many
// times it ran. Without it, Start costs a load and a branch.
//
// Phases may run on several goroutines at once, so the totals are CPU
// time spent in each phase, not a timeline; Wall is the build's own
// elapsed time.
package timing

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Enabled reports whether phases are being recorded.
var Enabled = os.Getenv("VSC_TIME") == "1"

var (
	mu     sync.Mutex
	phases = map[string]*phase{}
	begun  = time.Now()
	counts sync.Map // name -> *atomic.Int64
)

type phase struct {
	total time.Duration
	n     int
	order int
}

// Start begins timing a phase; call the returned function when it ends.
//
//	defer timing.Start("check")()
func Start(name string) func() {
	if !Enabled {
		return func() {}
	}
	t := time.Now()
	return func() { Add(name, time.Since(t)) }
}

// Add records d against a phase.
func Add(name string, d time.Duration) {
	if !Enabled {
		return
	}
	mu.Lock()
	p := phases[name]
	if p == nil {
		p = &phase{order: len(phases)}
		phases[name] = p
	}
	p.total += d
	p.n++
	mu.Unlock()
}

// Count adds n to a named counter: functions lowered, cache hits.
func Count(name string, n int) {
	if !Enabled {
		return
	}
	c, _ := counts.LoadOrStore(name, new(atomic.Int64))
	c.(*atomic.Int64).Add(int64(n))
}

// Print writes the totals, in the order the phases first ran.
func Print(w io.Writer) {
	if !Enabled {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	names := make([]string, 0, len(phases))
	for n := range phases {
		names = append(names, n)
	}
	sort.Slice(names, func(i, j int) bool { return phases[names[i]].order < phases[names[j]].order })
	fmt.Fprintf(w, "vsc: timing (phase totals across modules; wall %s)\n", ms(time.Since(begun)))
	for _, n := range names {
		p := phases[n]
		fmt.Fprintf(w, "  %-24s %10s  x%d\n", n, ms(p.total), p.n)
	}
	var cn []string
	counts.Range(func(k, _ any) bool { cn = append(cn, k.(string)); return true })
	sort.Strings(cn)
	for _, n := range cn {
		c, _ := counts.Load(n)
		fmt.Fprintf(w, "  %-24s %10d\n", n, c.(*atomic.Int64).Load())
	}
}

func ms(d time.Duration) string { return fmt.Sprintf("%.1fms", float64(d.Microseconds())/1000) }

// WriteFile saves the phases recorded so far, for a parent process to
// Merge: a worker compiling one package reports what it spent.
func WriteFile(path string) {
	if !Enabled {
		return
	}
	var b strings.Builder
	mu.Lock()
	for n, p := range phases {
		fmt.Fprintf(&b, "p\t%s\t%d\t%d\n", n, int64(p.total), p.n)
	}
	mu.Unlock()
	counts.Range(func(k, v any) bool {
		fmt.Fprintf(&b, "c\t%s\t%d\n", k.(string), v.(*atomic.Int64).Load())
		return true
	})
	os.WriteFile(path, []byte(b.String()), 0o644)
}

// Merge adds what a worker's WriteFile saved.
func Merge(path string) {
	if !Enabled {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		f := strings.Split(line, "\t")
		switch {
		case len(f) == 4 && f[0] == "p":
			d, _ := strconv.ParseInt(f[2], 10, 64)
			n, _ := strconv.Atoi(f[3])
			mu.Lock()
			p := phases[f[1]]
			if p == nil {
				p = &phase{order: len(phases)}
				phases[f[1]] = p
			}
			p.total += time.Duration(d)
			p.n += n
			mu.Unlock()
		case len(f) == 3 && f[0] == "c":
			n, _ := strconv.Atoi(f[2])
			Count(f[1], n)
		}
	}
}
