package build

import (
	"bytes"
	"debug/elf"
	"fmt"
	"sort"
	"strings"

	"github.com/vertex-language/ir"

	elfcore "github.com/vertex-language/elf"
	"github.com/vertex-language/elf/ar"
	elflink "github.com/vertex-language/elf/link"
)

// Android links against the device's own bionic, which nothing on the host
// has to provide. Each system library an image needs is stood in for by a
// stub linked on the spot: a shared object with the library's soname and a
// definition of every symbol the link leaves undefined. The dynamic
// linker searches every DT_NEEDED library for a name, so it does not
// matter which stub a symbol is in; libc's has them all and the others
// are there for their DT_NEEDED entries.

// androidInterp is bionic's dynamic linker for 64-bit programs.
const androidInterp = "/system/bin/linker64"

// androidPageSize is the page size images are laid out for: 16 KB, which
// Android 15 devices require and 4 KB ones accept.
const androidPageSize = 16384

// androidSystemLibs are the platform's NDK libraries, which -l names are
// stubbed for rather than looked for on disk.
var androidSystemLibs = map[string]bool{
	"c": true, "m": true, "dl": true, "log": true, "android": true,
	"EGL": true, "GLESv1_CM": true, "GLESv2": true, "GLESv3": true,
	"vulkan": true, "jnigraphics": true, "OpenSLES": true, "aaudio": true,
	"mediandk": true, "camera2ndk": true, "nativewindow": true, "z": true,
	"sync": true, "binder_ndk": true, "amidi": true, "icu": true,
}

// androidStart is the program's entry: the kernel's initial stack goes to
// the runtime (vertex_android_raw_args, for the arguments) and to
// __libc_init, which sets bionic up and calls main. The structors are
// empty: the dynamic linker has already run the program's initializers.
func androidStart(main string) string {
	return "\t.text\n\t.globl _start\n\t.type _start, @function\n\t.p2align 2\n_start:\n" +
		"\tmov x0, sp\n" +
		"\tadrp x1, vertex_android_raw_args\n" +
		"\tadd x1, x1, :lo12:vertex_android_raw_args\n" +
		"\tstr x0, [x1]\n" +
		"\tmov x1, xzr\n" +
		"\tadrp x2, " + main + "\n" +
		"\tadd x2, x2, :lo12:" + main + "\n" +
		"\tadrp x3, .Lvertex_structors\n" +
		"\tadd x3, x3, :lo12:.Lvertex_structors\n" +
		"\tb __libc_init\n" +
		"\t.data\n\t.p2align 3\n.Lvertex_structors:\n\t.quad 0\n\t.quad 0\n\t.quad 0\n"
}

// asmObject assembles text into an object for target.
func asmObject(target ir.Target, name, text string) (Input, error) {
	m := ir.NewModule(name, target)
	m.Asm(text)
	obj, err := Object(m, Options{})
	if err != nil {
		return Input{}, fmt.Errorf("%w: %s: %w", ErrLink, name, err)
	}
	return Input{Name: name + ".o", Data: obj}, nil
}

func aarch64AndroidELF(objs []Input, opts LinkOptions) ([]byte, error) {
	target, err := elfcore.ParseTarget("aarch64-linux-android")
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrLink, err)
	}

	inputs := append([]Input(nil), objs...)
	if !opts.Freestanding && !opts.NoRuntime {
		rt, err := Runtime(opts.Target)
		if err != nil {
			return nil, err
		}
		inputs = append(inputs, rt)
	}
	if !opts.Shared && !opts.Freestanding {
		main := opts.Entry
		if main == "" {
			main = "main"
		}
		start, err := asmObject(opts.Target, "vertex_start", androidStart(main))
		if err != nil {
			return nil, err
		}
		inputs = append(inputs, start)
	}

	// -l names: the platform's are stubbed, anything else is found on disk.
	var system []string
	var other []string
	for _, n := range opts.LibNames {
		if androidSystemLibs[n] {
			system = append(system, n)
		} else {
			other = append(other, n)
		}
	}
	found := make([]Input, 0, len(other))
	for _, n := range other {
		lib, err := findLibrary(opts.Target, n, opts.libraryDirs(opts.Target))
		if err != nil {
			return nil, err
		}
		found = append(found, lib)
	}
	archives := append(append([]Input(nil), opts.Libs...), found...)

	l := elflink.New(target)
	o := l.Options()
	o.MaxPageSize = androidPageSize
	o.CommonPageSize = androidPageSize
	switch {
	case opts.Freestanding:
		o.Output = elflink.OutputExec
		o.Static = true
		entry := opts.Entry
		if entry == "" {
			entry = "main"
		}
		o.Entry = entry
	case opts.Shared:
		o.Output = elflink.OutputShared
		o.SOName = opts.SOName
	default:
		o.Output = elflink.OutputPIE
		o.Interp = androidInterp
		o.Entry = "_start"
	}

	for _, in := range inputs {
		if err := l.AddFile(in.Name, in.Data); err != nil {
			return nil, fmt.Errorf("%w: %s: %w", ErrLink, in.Name, err)
		}
	}
	for _, in := range archives {
		if err := l.AddFile(in.Name, in.Data); err != nil {
			return nil, fmt.Errorf("%w: %s: %w", ErrLink, in.Name, err)
		}
	}

	if !opts.Freestanding {
		undefined, err := undefinedSymbols(append(append([]Input(nil), inputs...), archives...))
		if err != nil {
			return nil, err
		}
		libs := append([]string{"c"}, system...)
		seen := map[string]bool{}
		for i, n := range libs {
			if seen[n] {
				continue
			}
			seen[n] = true
			var syms []string
			if i == 0 {
				syms = undefined
			}
			stub, err := androidStub(opts.Target, target, "lib"+n+".so", syms)
			if err != nil {
				return nil, err
			}
			if err := l.AddShared("lib"+n+".so", stub); err != nil {
				return nil, fmt.Errorf("%w: lib%s.so stub: %w", ErrLink, n, err)
			}
		}
	}

	img, err := l.Link()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrLink, err)
	}
	return img.Bytes(), nil
}

// androidStub links a shared object named soname that defines each of syms.
func androidStub(t ir.Target, target elfcore.Target, soname string, syms []string) ([]byte, error) {
	var text strings.Builder
	text.WriteString("\t.text\n")
	for _, s := range syms {
		fmt.Fprintf(&text, "\t.globl %s\n\t.type %s, @function\n%s:\n\tret\n", s, s, s)
	}
	if len(syms) == 0 {
		text.WriteString("\tret\n")
	}
	obj, err := asmObject(t, strings.TrimSuffix(soname, ".so")+"_stub", text.String())
	if err != nil {
		return nil, err
	}
	l := elflink.New(target)
	o := l.Options()
	o.Output = elflink.OutputShared
	o.SOName = soname
	o.MaxPageSize = androidPageSize
	o.CommonPageSize = androidPageSize
	if err := l.AddObject(obj.Name, obj.Data); err != nil {
		return nil, fmt.Errorf("%w: %s stub: %w", ErrLink, soname, err)
	}
	img, err := l.Link()
	if err != nil {
		return nil, fmt.Errorf("%w: %s stub: %w", ErrLink, soname, err)
	}
	return img.Bytes(), nil
}

// undefinedSymbols lists the global symbols the inputs reference and none
// of them defines, sorted. Archives count whole: a member the link does
// not pull in adds at most a name to libc's stub that nothing uses.
func undefinedSymbols(inputs []Input) ([]string, error) {
	defined := map[string]bool{}
	wanted := map[string]bool{}
	scan := func(name string, data []byte) error {
		f, err := elf.NewFile(bytes.NewReader(data))
		if err != nil {
			return fmt.Errorf("%w: %s: %w", ErrLink, name, err)
		}
		syms, err := f.Symbols()
		if err != nil && err != elf.ErrNoSymbols {
			return fmt.Errorf("%w: %s: %w", ErrLink, name, err)
		}
		for _, s := range syms {
			bind := elf.ST_BIND(s.Info)
			if s.Name == "" || (bind != elf.STB_GLOBAL && bind != elf.STB_WEAK) {
				continue
			}
			if s.Section == elf.SHN_UNDEF {
				if bind == elf.STB_GLOBAL {
					wanted[s.Name] = true
				}
			} else {
				defined[s.Name] = true
			}
		}
		return nil
	}
	for _, in := range inputs {
		if ar.Is(in.Data) {
			r, err := ar.NewReader(bytes.NewReader(in.Data))
			if err != nil {
				return nil, fmt.Errorf("%w: %s: %w", ErrLink, in.Name, err)
			}
			for _, m := range r.Members {
				data, err := m.Data()
				if err != nil {
					return nil, fmt.Errorf("%w: %s(%s): %w", ErrLink, in.Name, m.Name, err)
				}
				if err := scan(in.Name+"("+m.Name+")", data); err != nil {
					return nil, err
				}
			}
			continue
		}
		if err := scan(in.Name, in.Data); err != nil {
			return nil, err
		}
	}
	var out []string
	for n := range wanted {
		// __start_/__stop_ are the linker's to define.
		if !defined[n] && !strings.HasPrefix(n, "__start_") && !strings.HasPrefix(n, "__stop_") {
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out, nil
}
