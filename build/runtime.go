package build

import (
	"fmt"
	"io/fs"
	"sync"

	"github.com/vertex-language/ir"
	"github.com/vertex-language/vcx"

	"github.com/vertex-language/vsc/build/buildcache"
	"github.com/vertex-language/vsc/stdlib"
	"github.com/vertex-language/vsc/timing"
)

// Runtime compiles and returns the in-tree C++ Vertex runtime object for target.
func Runtime(target ir.Target) (Input, error) {
	unit, ok := stdlib.RuntimeUnit(targetName(target))
	if !ok {
		return Input{}, fmt.Errorf("%w: runtime: no platform layer for %s", ErrTarget, target.Use())
	}
	return compileRuntime(target, unit, "vertex_runtime.o", true)
}

// RuntimeForSwift is the runtime for a program linked with Swift's own:
// the same, less the standard library's protocol descriptors, which
// libswiftCore defines. The runtime's stand-ins would take their names in
// the link and Swift's runtime would read them.
func RuntimeForSwift(target ir.Target) (Input, error) {
	unit, ok := stdlib.RuntimeUnit(targetName(target))
	if !ok {
		return Input{}, fmt.Errorf("%w: runtime: no platform layer for %s", ErrTarget, target.Use())
	}
	return compileRuntimeDefining(target, unit, "vertex_runtime.o", true, []string{"VERTEX_WITH_SWIFT_RUNTIME=1"})
}

// SwiftBridge compiles and returns the runtime bridge object for Swift String/Array interop.
func SwiftBridge(target ir.Target) (Input, error) {
	unit, ok := stdlib.SwiftBridgeUnit(targetName(target))
	if !ok {
		return Input{}, fmt.Errorf("%w: no Swift runtime to bridge to on %s", ErrTarget, target.Use())
	}
	return compileRuntime(target, unit, "vertex_swift_bridge.o", false)
}

// GPURuntime compiles the gpu runtime unit: devices, buffers and launches
// for the built-in gpu module, and its intrinsics for the CPU device.
// It is linked only into a program that imports gpu (GUIDELINES.md §1.1).
func GPURuntime(target ir.Target) (Input, error) {
	asm, fibers := stdlib.GPUAsm(targetName(target))
	var defs []string
	if fibers {
		defs = []string{"VERTEX_GPU_FIBERS=1"}
	}
	return compileRuntimeWith(target, "gpu/gpu.cpp", "vertex_gpu_runtime.o", false, defs, asm)
}

// compileRuntime compiles one translation unit of stdlib's runtime.
func compileRuntime(target ir.Target, unit, object string, asm bool) (Input, error) {
	return compileRuntimeDefining(target, unit, object, asm, nil)
}

// compileRuntimeDefining is compileRuntime with macros defined for the unit.
func compileRuntimeDefining(target ir.Target, unit, object string, asm bool, defs []string) (Input, error) {
	return compileRuntimeWith(target, unit, object, asm, defs, "")
}

// runtimes are the runtime objects this process has already produced, by
// cache key: a test binary links hundreds of programs against one.
// runtimes is each runtime unit this process has compiled, or is
// compiling, by key: one compile however many ask, WarmRuntime among them.
var runtimes sync.Map // buildcache.Key -> *runtimeOnce

type runtimeOnce struct {
	once sync.Once
	data []byte
	err  error
}

func compileRuntimeWith(target ir.Target, unit, object string, asm bool, defs []string, own string) (Input, error) {
	key := buildcache.New("runtime").
		String(target.String()).
		String(unit).
		String(fmt.Sprint(asm)).
		Strings(defs).
		String(own).
		Key()
	defer timing.Start("runtime")()
	v, _ := runtimes.LoadOrStore(key, &runtimeOnce{})
	r := v.(*runtimeOnce)
	r.once.Do(func() {
		if data, ok := buildcache.Get(key); ok {
			timing.Count("cache hit: runtime", 1)
			r.data = data
			return
		}
		in, err := buildRuntime(target, unit, object, asm, defs, own)
		if err != nil {
			r.err = err
			return
		}
		r.data = in.Data
		buildcache.Put(key, in.Data)
	})
	if r.err != nil {
		return Input{}, r.err
	}
	return Input{Name: object, Data: r.data}, nil
}

// WarmRuntime starts compiling target's runtime, which every program links
// and none changes: a cold build otherwise compiled it last, in the link,
// after everything else was done. The link takes it from here.
func WarmRuntime(target ir.Target) {
	go Runtime(target)
}

// buildRuntime compiles one runtime unit through vcx.
func buildRuntime(target ir.Target, unit, object string, asm bool, defs []string, own string) (Input, error) {
	src := stdlib.Runtime()
	text, err := fs.ReadFile(src, unit)
	if err != nil {
		return Input{}, fmt.Errorf("%w: runtime: %w", ErrLink, err)
	}
	include, err := fs.Sub(src, "include")
	if err != nil {
		return Input{}, fmt.Errorf("%w: runtime: %w", ErrLink, err)
	}

	c := &vcx.Compiler{
		Target:       targetName(target),
		Std:          vcx.Cxx23,
		Freestanding: true,
		Defs:         defs,
		IncludeFS:    []vcx.SystemInclude{{Name: "<vertex>", FS: include}},
		MinOS:        defaultMinOS,
	}
	m, diags, err := c.IR(vcx.Input{Name: unit, Data: text, FS: src})
	if err == nil && vcx.HasErrors(diags) {
		err = &vcx.DiagnosticError{Diagnostics: diags}
	}
	if err != nil {
		return Input{}, fmt.Errorf("%w: runtime: %w", ErrLink, err)
	}
	// Entering a continuation loads registers no C++ names, and the
	// runtime's own suspending primitives are entered the way an async
	// function is. Both are assembly, in the runtime's own object so
	// that every link of the runtime has them. See stdlib.TaskAsm and
	// stdlib/runtime/task.cpp.
	if asm {
		if text, ok := stdlib.TaskAsm(targetName(target)); ok {
			m.Asm(text)
		}
	}
	if own != "" {
		m.Asm(own)
	}
	obj, err := Object(m, Options{})
	if err != nil {
		return Input{}, fmt.Errorf("%w: runtime: %w", ErrLink, err)
	}
	return Input{Name: object, Data: obj}, nil
}
