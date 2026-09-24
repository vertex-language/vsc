package vsc_test

// The kernel ladder: tests/kernel/NNN-*.vs, one small thing a kernel does
// per file, climbing from an empty kernel to programs (see
// tests/kernel/README.md).
//
// There is no swiftc to ask here, so each file says what it prints:
//
//	// want: [5.0, 5.0]     a line of standard output, in order
//	// error: a kernel      the build fails, saying this
//
// Every file that builds is run twice, on the CPU device and on Metal
// (VERTEX_GPU=cpu, =metal), and both must print what it wants: the CPU
// device is the oracle the GPU is held to, and a file the two disagree
// on names the kernel. On a machine with no Metal the second run is the
// CPU again, which the runtime says on standard error.

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/vertex-language/vsc/internal/cli"
)

type kernelSpec struct {
	want    []string
	errText []string
}

func readKernelSpec(t *testing.T, path string) kernelSpec {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var s kernelSpec
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "// want:"):
			s.want = append(s.want, strings.TrimSpace(strings.TrimPrefix(line, "// want:")))
		case line == "// want:":
			s.want = append(s.want, "")
		case strings.HasPrefix(line, "// error:"):
			s.errText = append(s.errText, strings.TrimSpace(strings.TrimPrefix(line, "// error:")))
		}
	}
	return s
}

func TestKernels(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("tests", "kernel", "*.vs"))
	if err != nil || len(files) == 0 {
		t.Skip("no kernels in tests/kernel")
	}
	sort.Strings(files)
	for _, path := range files {
		name := strings.TrimSuffix(filepath.Base(path), ".vs")
		t.Run(name, func(t *testing.T) {
			spec := readKernelSpec(t, path)
			if len(spec.want) == 0 && len(spec.errText) == 0 {
				t.Fatal("the file says neither what it prints (// want:) nor what it is refused with (// error:)")
			}
			bin := filepath.Join(t.TempDir(), name)
			var stdout, stderr bytes.Buffer
			code := cli.Run([]string{"build", "-o", bin, path}, &stdout, &stderr)
			if len(spec.errText) > 0 {
				if code == 0 {
					t.Fatalf("built, and should have been refused with %q", spec.errText)
				}
				for _, want := range spec.errText {
					if !strings.Contains(stderr.String(), want) {
						t.Errorf("refused, but without %q:\n%s", want, stderr.String())
					}
				}
				return
			}
			if code != 0 {
				t.Fatalf("vsc refused it:\n%s", stderr.String())
			}
			// A kernel not built for a device says so, and runs where it
			// can instead: which is a failure here, where every kernel is
			// meant to be built for every device.
			if stderr.Len() > 0 {
				t.Errorf("vsc said something building it:\n%s", stderr.String())
			}
			want := strings.Join(spec.want, "\n") + "\n"
			for _, device := range []string{"cpu", "metal"} {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				cmd := exec.CommandContext(ctx, bin)
				cmd.Env = append(os.Environ(), "VERTEX_GPU="+device)
				var out, errOut bytes.Buffer
				cmd.Stdout, cmd.Stderr = &out, &errOut
				err := cmd.Run()
				cancel()
				if err != nil {
					t.Errorf("on %s: %v\n%s%s", device, err, out.String(), errOut.String())
					continue
				}
				if out.String() != want {
					t.Errorf("on %s:\n--- got ---\n%s--- want ---\n%s", device, out.String(), want)
				}
			}
		})
	}
}
