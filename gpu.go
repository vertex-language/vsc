package vsc

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/vertex-language/vsc/stdlib"
)

// GPUModule is the import path the compiler answers itself: the built-in
// gpu module, whose device half is intrinsics and whose host half calls
// the gpu runtime unit (proposed_vertex_kernel.md §3). A folder or a
// repository of that name is never asked.
const GPUModule = "gpu"

var gpuDir struct {
	once sync.Once
	dir  string
	err  error
}

// GPUSourceDir is a directory holding the built-in gpu module's source,
// as the compiler carries it: written once under the package cache, in a
// folder named by what is in it, so that two compilers with different
// gpu modules never share one.
func GPUSourceDir() (string, error) {
	gpuDir.once.Do(func() {
		gpuDir.dir, gpuDir.err = materializeGPU()
	})
	return gpuDir.dir, gpuDir.err
}

func materializeGPU() (string, error) {
	src := stdlib.GPU()
	var names []string
	files := map[string][]byte{}
	err := fs.WalkDir(src, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(src, path)
		if err != nil {
			return err
		}
		names = append(names, path)
		files[path] = data
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("the built-in gpu module: %w", err)
	}
	sort.Strings(names)
	h := sha256.New()
	for _, n := range names {
		h.Write([]byte(n))
		h.Write([]byte{0})
		h.Write(files[n])
		h.Write([]byte{0})
	}
	dir := filepath.Join(builtinCacheDir(), "builtin", "gpu-"+hex.EncodeToString(h.Sum(nil))[:16])
	if _, err := os.Stat(filepath.Join(dir, ".complete")); err == nil {
		return dir, nil
	}
	tmp, err := os.MkdirTemp(filepath.Dir(dir), "gpu-tmp-")
	if err != nil {
		if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
			return "", fmt.Errorf("the built-in gpu module: %w", err)
		}
		if tmp, err = os.MkdirTemp(filepath.Dir(dir), "gpu-tmp-"); err != nil {
			return "", fmt.Errorf("the built-in gpu module: %w", err)
		}
	}
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(tmp, n), files[n], 0o644); err != nil {
			os.RemoveAll(tmp)
			return "", fmt.Errorf("the built-in gpu module: %w", err)
		}
	}
	if err := os.WriteFile(filepath.Join(tmp, ".complete"), nil, 0o644); err != nil {
		os.RemoveAll(tmp)
		return "", fmt.Errorf("the built-in gpu module: %w", err)
	}
	if err := os.Rename(tmp, dir); err != nil {
		os.RemoveAll(tmp)
		// Another compile got there first: its copy is the same bytes.
		if _, statErr := os.Stat(filepath.Join(dir, ".complete")); statErr == nil {
			return dir, nil
		}
		return "", fmt.Errorf("the built-in gpu module: %w", err)
	}
	return dir, nil
}

// ImportsGPU reports whether a compile's packages include the built-in
// gpu module, which is what links the gpu runtime unit.
func ImportsGPU(pkgs []Package) bool {
	for _, p := range pkgs {
		if p.Name == GPUModule {
			if dir, err := GPUSourceDir(); err == nil && p.Dir == dir {
				return true
			}
		}
	}
	return false
}

// builtinCacheDir is the package cache importer.CacheDir names --
// VERTEXCACHE, or "vertex" in this machine's cache -- found without
// importing the fetcher, which the compiler proper does not need.
func builtinCacheDir() string {
	if dir := os.Getenv("VERTEXCACHE"); dir != "" {
		return dir
	}
	if base, err := os.UserCacheDir(); err == nil {
		return filepath.Join(base, "vertex")
	}
	return filepath.Join(os.TempDir(), "vertex-cache")
}
