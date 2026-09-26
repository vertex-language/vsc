package cli

// Imported packages compiled in parallel, one worker process each.
//
// A package's compile reads its imports' source, not their objects, so
// every package the cache does not have can be compiled at once. They
// are compiled in processes rather than goroutines because the checker
// is not safe to run twice in one process -- it installs the module it
// is checking in package-level state (types.BuiltinAssoc) -- and a
// process per package is how `go build` runs its compiler too.
//
// A worker is this executable run as `vsc __compile-package JOB OUT`:
// JOB is the package and everything the build's flags say about finding
// its imports, OUT where its object goes, and OUT.time its phase times.

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"sync"

	"github.com/vertex-language/vsc"
	"github.com/vertex-language/vsc/timing"
)

// packageJob is one worker's input.
type packageJob struct {
	Pkg     vsc.Package
	MainDir string
	Target  string
	MinOS   string
	Include []string
	Pkgs    []string
	Replace []string
	Offline bool
}

// Workers says this process may run packages in worker processes: it is
// the vsc command, which cmd/vsc says by setting it. Anything else that
// calls Run -- a test binary above all -- compiles in-process, because
// re-running its own executable would not start a worker. A test binary
// ignores the arguments and runs every test again, each of which starts
// more of itself.
var Workers bool

// workerEnv marks a worker process, which never starts workers itself.
const workerEnv = "VSC_WORKER"

// jobs is how many packages compile at once: VSC_JOBS, or one per CPU,
// and one wherever workers cannot be started.
func jobs() int {
	if !Workers || os.Getenv(workerEnv) == "1" {
		return 1
	}
	if n, err := strconv.Atoi(os.Getenv("VSC_JOBS")); err == nil && n > 0 {
		return n
	}
	return runtime.NumCPU()
}

// compilePackages compiles the packages at idx, in parallel where there
// is more than one, and returns their objects by index. A package that
// fails has printed why; its exit code is returned.
func compilePackages(pkgs []vsc.Package, idx []int, bf *buildFlags, mainDir, minOS string, stderr io.Writer) (map[int][]byte, int) {
	out := map[int][]byte{}
	n := min(jobs(), len(idx))
	if n <= 1 {
		target, _ := bf.resolve()
		for _, i := range idx {
			obj, code := buildPackage(pkgs[i], bf, target, minOS, stderr)
			if code != exitOK {
				return nil, code
			}
			out[i] = obj
		}
		return out, exitOK
	}

	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintln(stderr, "vsc:", err)
		return nil, exitUsage
	}
	dir, err := os.MkdirTemp("", "vsc-pkg-")
	if err != nil {
		fmt.Fprintln(stderr, "vsc:", err)
		return nil, exitUsage
	}
	defer os.RemoveAll(dir)

	// Largest first: the build ends when its longest package does, so
	// that one should not be the last to start.
	order := slices.Clone(idx)
	size := func(i int) int {
		s := 0
		for _, src := range pkgs[i].Sources {
			s += len(src.Text)
		}
		return s
	}
	slices.SortStableFunc(order, func(a, b int) int { return size(b) - size(a) })

	var mu sync.Mutex
	errw := &lockedWriter{w: stderr}
	worst := exitOK
	work := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < n; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range work {
				obj, code := runWorker(exe, dir, i, packageJob{
					Pkg:     pkgs[i],
					MainDir: mainDir,
					Target:  bf.target,
					MinOS:   minOS,
					Include: bf.include,
					Pkgs:    bf.pkgs,
					Replace: bf.replace,
					Offline: bf.offline,
				}, errw)
				mu.Lock()
				if code != exitOK {
					worst = max(worst, code)
				} else {
					out[i] = obj
				}
				mu.Unlock()
			}
		}()
	}
	done := timing.Start("packages (parallel wall)")
	for _, i := range order {
		work <- i
	}
	close(work)
	wg.Wait()
	done()
	if worst != exitOK {
		return nil, worst
	}
	return out, exitOK
}

func runWorker(exe, dir string, i int, job packageJob, stderr io.Writer) ([]byte, int) {
	jobPath := filepath.Join(dir, fmt.Sprintf("%d.job", i))
	objPath := filepath.Join(dir, fmt.Sprintf("%d.o", i))
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(job); err != nil {
		fmt.Fprintln(stderr, "vsc:", err)
		return nil, exitUsage
	}
	if err := os.WriteFile(jobPath, buf.Bytes(), 0o644); err != nil {
		fmt.Fprintln(stderr, "vsc:", err)
		return nil, exitUsage
	}
	cmd := exec.Command(exe, "__compile-package", jobPath, objPath)
	cmd.Env = append(os.Environ(), workerEnv+"=1")
	var diag bytes.Buffer
	cmd.Stderr = &diag
	err := cmd.Run()
	if diag.Len() > 0 {
		stderr.Write(diag.Bytes())
	}
	timing.Merge(objPath + ".time")
	if err != nil {
		if exit, ok := err.(*exec.ExitError); ok && exit.ExitCode() > 0 {
			return nil, exit.ExitCode()
		}
		fmt.Fprintf(stderr, "vsc: package %s: %v\n", job.Pkg.Name, err)
		return nil, exitUsage
	}
	obj, err := os.ReadFile(objPath)
	if err != nil {
		fmt.Fprintf(stderr, "vsc: package %s: %v\n", job.Pkg.Name, err)
		return nil, exitUsage
	}
	return obj, exitOK
}

// cmdCompilePackage is a worker: one package, from a job file, to an
// object file.
func cmdCompilePackage(args []string, stderr io.Writer) int {
	if os.Getenv(workerEnv) != "1" {
		fmt.Fprintln(stderr, "vsc: __compile-package is run by vsc itself, not by hand")
		return exitUsage
	}
	if len(args) != 2 {
		fmt.Fprintln(stderr, "vsc: __compile-package JOB OUT")
		return exitUsage
	}
	data, err := os.ReadFile(args[0])
	if err != nil {
		fmt.Fprintln(stderr, "vsc:", err)
		return exitUsage
	}
	var job packageJob
	if err := gob.NewDecoder(bytes.NewReader(data)).Decode(&job); err != nil {
		fmt.Fprintln(stderr, "vsc:", err)
		return exitUsage
	}
	bf := &buildFlags{common: common{
		target:  job.Target,
		include: job.Include,
		pkgs:    job.Pkgs,
		replace: job.Replace,
		offline: job.Offline,
	}}
	target, err := bf.resolve()
	if err != nil {
		fmt.Fprintln(stderr, "vsc:", err)
		return exitUsage
	}
	bf.mainModule(job.MainDir)
	obj, code := buildPackage(job.Pkg, bf, target, job.MinOS, stderr)
	timing.WriteFile(args[1] + ".time")
	if code != exitOK {
		return code
	}
	if err := os.WriteFile(args[1], obj, 0o644); err != nil {
		fmt.Fprintln(stderr, "vsc:", err)
		return exitUsage
	}
	return exitOK
}

// lockedWriter serializes writes from the workers' goroutines.
type lockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}
