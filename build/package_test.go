package build_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/vertex-language/vsc/build"
	"github.com/vertex-language/vsc/pkg"
)

// TestPackagesMatchSwiftPM builds every package in tests/packages twice --
// with `swift build`, and with this compiler's package build -- runs each
// program both produce, and compares what they print.
//
// SwiftPM is the oracle for everything a package is: which targets, in
// which order, compiled with which flags, importing which headers, linked
// into which programs. A package this build gets wrong prints something
// else or does not build.
func TestPackagesMatchSwiftPM(t *testing.T) {
	if runtime.GOARCH != "arm64" || runtime.GOOS != "darwin" {
		t.Skip("not on Apple Silicon")
	}
	swift, err := exec.LookPath("swift")
	if err != nil {
		t.Skip("no swift on PATH")
	}
	target, ok := build.Host()
	if !ok {
		t.Skip("no backend for this machine")
	}
	dirs, _ := filepath.Glob("../tests/packages/*")
	ran := 0
	for _, dir := range dirs {
		if _, err := os.Stat(filepath.Join(dir, pkg.ManifestName)); err != nil {
			continue
		}
		ran++
		t.Run(filepath.Base(dir), func(t *testing.T) {
			scratch := t.TempDir()
			if out, err := exec.Command(swift, "build", "--package-path", dir, "--scratch-path", scratch).CombinedOutput(); err != nil {
				t.Fatalf("swift build: %v\n%s", err, out)
			}

			m, diags, err := pkg.Load(dir)
			if err != nil {
				t.Fatal(err)
			}
			for _, d := range diags {
				t.Fatalf("manifest: %s", d.Message)
			}
			p, err := pkg.Resolve(dir, m, "macos", "debug")
			if err != nil {
				t.Fatal(err)
			}
			work := t.TempDir()
			products, err := build.BuildPackage(p, build.PackageOptions{Target: target, Work: work})
			if err != nil {
				t.Fatal(err)
			}
			if len(products) == 0 {
				t.Fatal("no programs built")
			}
			for _, prod := range products {
				want, err := exec.Command(filepath.Join(scratch, "debug", prod.Name)).Output()
				if err != nil {
					t.Fatalf("SwiftPM's %s: %v", prod.Name, err)
				}
				bin := filepath.Join(work, prod.Name)
				if err := os.WriteFile(bin, prod.Image, 0o755); err != nil {
					t.Fatal(err)
				}
				got, err := exec.Command(bin).CombinedOutput()
				if err != nil {
					t.Fatalf("%s: %v\n%s", prod.Name, err, got)
				}
				if string(got) != string(want) {
					t.Errorf("%s printed:\n%s\nSwiftPM's printed:\n%s", prod.Name, got, want)
				}
			}
		})
	}
	if ran == 0 {
		t.Fatal("no packages in tests/packages")
	}
}
