package cli

// Imported packages compiled in parallel, in a pool of worker processes.
//
// A package's compile reads its imports' source, not their objects, so
// every package the cache does not have can be compiled at once. They
// are compiled in processes rather than goroutines because the checker
// is not safe to run twice in one process -- it installs the module it
// is checking in package-level state (types.BuiltinAssoc) -- and a
// process per package is how `go build` runs its compiler too.
//
// A worker is this executable run as `vsc __compile-package TIME`. It
// compiles the jobs it is sent, one at a time, for the whole build --
// each job is a package and everything the build's flags say about
// finding its imports -- and writes its phase times to TIME at the end.

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
	fail := func(code int) {
		mu.Lock()
		worst = max(worst, code)
		mu.Unlock()
	}
	work := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < n; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			wk, err := startWorker(exe, filepath.Join(dir, fmt.Sprintf("w%d.time", w)))
			if err != nil {
				fmt.Fprintln(errw, "vsc:", err)
				fail(exitUsage)
				for range work {
				}
				return
			}
			defer func() {
				if err := wk.stop(); err != nil {
					fmt.Fprintln(errw, "vsc: worker:", err)
					fail(exitUsage)
				}
			}()
			for i := range work {
				res, err := wk.run(packageJob{
					Pkg:     pkgs[i],
					MainDir: mainDir,
					Target:  bf.target,
					MinOS:   minOS,
					Include: bf.include,
					Pkgs:    bf.pkgs,
					Replace: bf.replace,
					Offline: bf.offline,
				})
				if err != nil {
					fmt.Fprintf(errw, "vsc: package %s: %v\n", pkgs[i].Name, err)
					fail(exitUsage)
					for range work {
					}
					return
				}
				if len(res.Diag) > 0 {
					errw.Write(res.Diag)
				}
				if res.Code != exitOK {
					fail(res.Code)
					continue
				}
				mu.Lock()
				out[i] = res.Obj
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
	for w := 0; w < n; w++ {
		timing.Merge(filepath.Join(dir, fmt.Sprintf("w%d.time", w)))
	}
	if worst != exitOK {
		return nil, worst
	}
	return out, exitOK
}

// packageResult is a worker's answer to one job.
type packageResult struct {
	Obj  []byte
	Code int
	Diag []byte // what compiling the package printed
}

// A worker is one process that compiles packages until its input ends.
type worker struct {
	cmd *exec.Cmd
	in  io.WriteCloser
	enc *gob.Encoder
	dec *gob.Decoder
}

// startWorker runs `vsc __compile-package TIME`, which reads jobs from
// its standard input and answers each on its standard output, then
// writes its phase times to TIME.
func startWorker(exe, timePath string) (*worker, error) {
	cmd := exec.Command(exe, "__compile-package", timePath)
	cmd.Env = append(os.Environ(), workerEnv+"=1")
	cmd.Stderr = os.Stderr
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &worker{cmd: cmd, in: in, enc: gob.NewEncoder(in), dec: gob.NewDecoder(out)}, nil
}

func (w *worker) run(job packageJob) (packageResult, error) {
	var res packageResult
	if err := w.enc.Encode(job); err != nil {
		return res, err
	}
	if err := w.dec.Decode(&res); err != nil {
		return res, fmt.Errorf("the worker stopped: %w", err)
	}
	return res, nil
}

// stop ends the worker's input and waits for it to exit.
func (w *worker) stop() error {
	w.in.Close()
	return w.cmd.Wait()
}

// cmdCompilePackage is a worker: package jobs from standard input,
// compiled one after another in this process, each answered on standard
// output. Keeping the process for the whole build is what lets its
// packages share what one compile loads anyway -- core, the SDK, the
// natives bound for the build.
func cmdCompilePackage(args []string, stderr io.Writer) int {
	if os.Getenv(workerEnv) != "1" {
		fmt.Fprintln(stderr, "vsc: __compile-package is run by vsc itself, not by hand")
		return exitUsage
	}
	if len(args) != 1 {
		fmt.Fprintln(stderr, "vsc: __compile-package TIME")
		return exitUsage
	}
	// The answers own standard output: anything a compile prints there
	// goes to standard error instead, not into the stream.
	answers := os.Stdout
	os.Stdout = os.Stderr
	dec := gob.NewDecoder(os.Stdin)
	enc := gob.NewEncoder(answers)
	var bf *buildFlags
	var flags packageJob // the flags bf was made for
	for {
		var job packageJob
		if err := dec.Decode(&job); err != nil {
			if err == io.EOF {
				break
			}
			fmt.Fprintln(stderr, "vsc:", err)
			return exitUsage
		}
		var diag bytes.Buffer
		res := packageResult{Code: exitOK}
		if bf == nil || !sameFlags(flags, job) {
			bf = &buildFlags{common: common{
				target:  job.Target,
				include: job.Include,
				pkgs:    job.Pkgs,
				replace: job.Replace,
				offline: job.Offline,
			}}
			flags = job
			bf.mainModule(job.MainDir)
		}
		target, err := bf.resolve()
		if err != nil {
			fmt.Fprintln(&diag, "vsc:", err)
			res.Code = exitUsage
		} else {
			res.Obj, res.Code = buildPackage(job.Pkg, bf, target, job.MinOS, &diag)
		}
		res.Diag = diag.Bytes()
		if err := enc.Encode(res); err != nil {
			fmt.Fprintln(stderr, "vsc:", err)
			return exitUsage
		}
	}
	timing.WriteFile(args[0])
	return exitOK
}

// sameFlags reports whether two jobs find their imports the same way.
func sameFlags(a, b packageJob) bool {
	return a.MainDir == b.MainDir && a.Target == b.Target && a.Offline == b.Offline &&
		slices.Equal(a.Include, b.Include) && slices.Equal(a.Pkgs, b.Pkgs) && slices.Equal(a.Replace, b.Replace)
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
