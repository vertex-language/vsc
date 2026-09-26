package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/vertex-language/ir"
	"github.com/vertex-language/vsc"
	"github.com/vertex-language/vsc/build/buildcache"
	"github.com/vertex-language/vsc/timing"
)

// A packageStream builds the imported packages as the importer finds
// them, rather than once it has found them all.
//
// The importer hands over each package after every package it imports
// (vsc.Options.OnPackage), so its key -- its source and its imports'
// surfaces -- can be made then. One the cache has is taken; one it has
// not goes to the worker processes at once, while the importer reads the
// rest and the program is checked. A folder's C++ starts compiling as its
// package is found, too: its bindings are made by then.
//
// A build with one package to compile still compiles it in this process
// once the imports are read, as before: starting a worker for it costs
// more than it saves, and an edit builds one package. The workers start
// with the second.
type packageStream struct {
	c       *common
	bf      *buildFlags
	target  ir.Target
	mainDir string
	minOS   string
	stderr  io.Writer

	pkgs     []vsc.Package
	keys     map[string]buildcache.Key
	surfaces map[string]buildcache.Key
	keyed    map[int]bool
	objs     map[int][]byte
	held     []int // misses not yet handed to a pool
	pool     *workerPool
}

func newPackageStream(c *common, bf *buildFlags, target ir.Target, mainDir, minOS string, stderr io.Writer) *packageStream {
	return &packageStream{
		c: c, bf: bf, target: target, mainDir: mainDir, minOS: minOS, stderr: stderr,
		keys: map[string]buildcache.Key{}, surfaces: map[string]buildcache.Key{},
		keyed: map[int]bool{}, objs: map[int][]byte{},
	}
}

// add is vsc.Options.OnPackage: it runs on the importer's goroutine.
func (s *packageStream) add(p vsc.Package) {
	i := len(s.pkgs)
	s.pkgs = append(s.pkgs, p)
	if p.Native && jobs() > 1 {
		s.c.startNative(p.Dir)
	}
	if key, ok := packageKey(p, s.target, s.minOS, s.surfaces); ok {
		s.keys[p.Dir] = key
		s.surfaces[p.Dir] = surfaceKey(p, s.surfaces)
		s.keyed[i] = true
		if obj, hit := buildcache.Get(key); hit {
			timing.Count("cache hit: package", 1)
			s.objs[i] = obj
			return
		}
	}
	timing.Count("cache miss: package", 1)
	if s.pool != nil {
		s.pool.submit(i, p)
		return
	}
	s.held = append(s.held, i)
	if len(s.held) >= 2 && jobs() > 1 {
		s.pool = newWorkerPool(s.bf, s.mainDir, s.minOS, s.stderr)
		for _, j := range s.held {
			s.pool.submit(j, s.pkgs[j])
		}
		s.held = nil
	}
}

// finish is every package's object, in the importer's order, once the
// imports are read: what the pool built, and the held packages built here.
func (s *packageStream) finish(pkgs []vsc.Package) ([][]byte, int) {
	if len(pkgs) != len(s.pkgs) {
		return nil, s.fail(fmt.Errorf("the importer handed over %d packages but found %d", len(s.pkgs), len(pkgs)))
	}
	for i := range pkgs {
		if pkgs[i].Dir != s.pkgs[i].Dir {
			return nil, s.fail(fmt.Errorf("the importer handed over %s where it found %s", s.pkgs[i].Dir, pkgs[i].Dir))
		}
	}
	built := map[int][]byte{}
	if s.pool != nil {
		out, code := s.pool.close()
		if code != exitOK {
			return nil, code
		}
		built = out
	}
	if len(s.held) > 0 {
		out, code := compilePackages(pkgs, s.held, s.bf, s.mainDir, s.minOS, s.stderr)
		if code != exitOK {
			return nil, code
		}
		for i, obj := range out {
			built[i] = obj
		}
	}
	objs := make([][]byte, len(pkgs))
	for i := range pkgs {
		if obj, ok := s.objs[i]; ok {
			objs[i] = obj
			continue
		}
		objs[i] = built[i]
		if s.keyed[i] {
			buildcache.Put(s.keys[pkgs[i].Dir], built[i])
		}
	}
	return objs, exitOK
}

func (s *packageStream) fail(err error) int {
	fmt.Fprintln(s.stderr, "vsc:", err)
	return exitUsage
}

// A workerPool compiles packages in worker processes as they are handed
// to it. A worker free takes the largest package waiting: the build ends
// when its longest package does, so that one should not start last.
type workerPool struct {
	bf      *buildFlags
	mainDir string
	minOS   string
	errw    *lockedWriter
	exe     string
	dir     string
	err     error

	mu      sync.Mutex
	cond    *sync.Cond
	queue   []poolJob
	closed  bool
	started int
	jobs    int
	out     map[int][]byte
	worst   int
	wg      sync.WaitGroup
	done    func()
}

type poolJob struct {
	i    int
	pkg  vsc.Package
	size int
}

func newWorkerPool(bf *buildFlags, mainDir, minOS string, stderr io.Writer) *workerPool {
	p := &workerPool{bf: bf, mainDir: mainDir, minOS: minOS, errw: &lockedWriter{w: stderr}, out: map[int][]byte{}}
	p.cond = sync.NewCond(&p.mu)
	p.exe, p.err = os.Executable()
	if p.err == nil {
		p.dir, p.err = os.MkdirTemp("", "vsc-pkg-")
	}
	p.done = timing.Start("packages (parallel wall)")
	return p
}

// submit queues package i, starting another worker while there are fewer
// than there are packages handed over and the machine has cores.
func (p *workerPool) submit(i int, pkg vsc.Package) {
	size := 0
	for _, src := range pkg.Sources {
		size += len(src.Text)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.queue = append(p.queue, poolJob{i, pkg, size})
	p.jobs++
	if p.started < jobs() && p.started < p.jobs {
		w := p.started
		p.started++
		p.wg.Add(1)
		go p.worker(w)
	}
	p.cond.Signal()
}

func (p *workerPool) fail(code int) {
	p.mu.Lock()
	p.worst = max(p.worst, code)
	p.mu.Unlock()
}

// next is the largest package waiting, or false once the pool is closed
// and none is.
func (p *workerPool) next() (poolJob, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for len(p.queue) == 0 && !p.closed {
		p.cond.Wait()
	}
	if len(p.queue) == 0 {
		return poolJob{}, false
	}
	best := 0
	for k, j := range p.queue {
		if j.size > p.queue[best].size {
			best = k
		}
	}
	j := p.queue[best]
	p.queue = append(p.queue[:best], p.queue[best+1:]...)
	return j, true
}

func (p *workerPool) worker(w int) {
	defer p.wg.Done()
	drain := func() {
		for {
			if _, ok := p.next(); !ok {
				return
			}
		}
	}
	if p.err != nil {
		fmt.Fprintln(p.errw, "vsc:", p.err)
		p.fail(exitUsage)
		drain()
		return
	}
	wk, err := startWorker(p.exe, filepath.Join(p.dir, fmt.Sprintf("w%d.time", w)))
	if err != nil {
		fmt.Fprintln(p.errw, "vsc:", err)
		p.fail(exitUsage)
		drain()
		return
	}
	defer func() {
		if err := wk.stop(); err != nil {
			fmt.Fprintln(p.errw, "vsc: worker:", err)
			p.fail(exitUsage)
		}
	}()
	for {
		j, ok := p.next()
		if !ok {
			return
		}
		res, err := wk.run(packageJob{
			Pkg:     j.pkg,
			MainDir: p.mainDir,
			Target:  p.bf.target,
			MinOS:   p.minOS,
			Include: p.bf.include,
			Pkgs:    p.bf.pkgs,
			Replace: p.bf.replace,
			Offline: p.bf.offline,
		})
		if err != nil {
			fmt.Fprintf(p.errw, "vsc: package %s: %v\n", j.pkg.Name, err)
			p.fail(exitUsage)
			drain()
			return
		}
		if len(res.Diag) > 0 {
			p.errw.Write(res.Diag)
		}
		if res.Code != exitOK {
			p.fail(res.Code)
			continue
		}
		p.mu.Lock()
		p.out[j.i] = res.Obj
		p.mu.Unlock()
	}
}

// close waits for every package handed over, and is what they built.
func (p *workerPool) close() (map[int][]byte, int) {
	p.mu.Lock()
	p.closed = true
	p.cond.Broadcast()
	p.mu.Unlock()
	p.wg.Wait()
	p.done()
	for w := 0; w < p.started; w++ {
		timing.Merge(filepath.Join(p.dir, fmt.Sprintf("w%d.time", w)))
	}
	if p.dir != "" {
		os.RemoveAll(p.dir)
	}
	if p.worst != exitOK {
		return nil, p.worst
	}
	return p.out, exitOK
}

// A nativeFuture is a folder's C++ objects, compiling since its package
// was found.
type nativeFuture struct {
	done chan struct{}
	r    nativeResult
}

// startNative starts compiling dir's C++ module, whose bindings the
// importer has made. It runs on the importer's goroutine, which is the
// one that writes c.natives.
func (c *common) startNative(dir string) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return
	}
	nat := c.natives[abs]
	if nat == nil {
		return
	}
	c.early.mu.Lock()
	if c.early.m == nil {
		c.early.m = map[string]*nativeFuture{}
	}
	if c.early.m[abs] != nil {
		c.early.mu.Unlock()
		return
	}
	f := &nativeFuture{done: make(chan struct{})}
	c.early.m[abs] = f
	c.early.mu.Unlock()
	go func() {
		defer close(f.done)
		doneNative := timing.Start("native C++ objects")
		f.r.objs, f.r.need, f.r.err = nat.n.Objects(nat.thunks, nativeWork())
		doneNative()
	}()
}

// earlyNative is the compile startNative began for dir, or nil.
func (c *common) earlyNative(dir string) *nativeFuture {
	c.early.mu.Lock()
	defer c.early.mu.Unlock()
	return c.early.m[dir]
}
