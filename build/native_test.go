package build

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/vertex-language/ir"
)

// TestNativeBindings reads a C++ module's exports into what a package's
// Vertex sees of them, and the thunks those declarations call.
func TestNativeBindings(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("reads the macOS SDK's headers")
	}
	dir := t.TempDir()
	iface := filepath.Join(dir, "codes.cpp")
	src := `module;
#pragma vertex framework("CoreFoundation")
#pragma vertex library("m")
#include <cstdint>
#include <string_view>
export module net.codes;

export enum class Code : int32_t { ok = 0, bad = -1, gone = -1 };
export enum Flag : uint32_t { read = 1, write = 2 };
export constexpr int64_t limit = 1 << 20;
export int32_t classify(Code c, std::string_view why, uint8_t* out) noexcept;
export namespace net::codes { double scale(double x) { return x; } }
export namespace detail { void reset() {} }
export template <class T> T twice(T x) { return x + x; }
export struct Point { int x, y; };
export Point origin() { return {}; }
export [[noreturn]] void quit(int32_t code) noexcept;
int32_t hidden(int32_t x) { return x; }
`
	if err := os.WriteFile(iface, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	n, err := FindNative(dir, []string{iface}, ir.AArch64MacOS, "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(n.Frameworks, ",") != "CoreFoundation" || strings.Join(n.Libraries, ",") != "m" {
		t.Errorf("links = %v %v", n.Frameworks, n.Libraries)
	}
	if n.Module != "net.codes" || n.Interface != iface {
		t.Fatalf("FindNative = %+v", n)
	}
	vs, thunks, skipped, err := n.Bindings("codes", true)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"package codes",
		"public enum Code: int32 {",
		"case bad = -1",
		"public enum Flag {",
		"public static let write: uint32 = 2",
		"public let limit: int64 = 1048576",
		"public func classify(_ c: Code, _ why: string, _ out: UnsafeMutablePointer<uint8>?) -> int32 {",
		"(c.rawValue, why, why.utf8.count, out)",
		"public func scale(_ x: float64) -> float64",
		"public enum detail {",
		"public static func reset()",
		// [[noreturn]] is Never, as ClangImporter imports it.
		"public func quit(_ code: int32) -> Never {\n    __vs_",
		") -> Never\n",
	} {
		if !strings.Contains(string(vs), want) {
			t.Errorf("the Vertex has no %q:\n%s", want, vs)
		}
	}
	for _, not := range []string{"hidden", "twice", "gone", "origin"} {
		if strings.Contains(string(vs), not) {
			t.Errorf("the Vertex has %q, which it should not:\n%s", not, vs)
		}
	}
	for _, want := range []string{
		"module net.codes;",
		"static_cast<::Code>(a0_0)",
		"std::string_view(a1_0, static_cast<std::size_t>(a1_1))",
		") noexcept {",
		"extern \"C\" [[noreturn]] void __vs_net_codes_quit_",
	} {
		if !strings.Contains(string(thunks), want) {
			t.Errorf("the thunks have no %q:\n%s", want, thunks)
		}
	}
	if !strings.Contains(strings.Join(skipped, "\n"), "origin") {
		t.Errorf("skipped = %v, want origin named", skipped)
	}
}

func TestModuleNameFor(t *testing.T) {
	for path, want := range map[string]string{
		"math":                    "math",
		"net/tcp":                 "net.tcp",
		"github.com/you/thing/x":  "thing.x",
		"github.com/you/my-thing": "my_thing",
		"image/format/png":        "image.format.png",
	} {
		if got := ModuleNameFor(path); got != want {
			t.Errorf("ModuleNameFor(%s) = %s, want %s", path, got, want)
		}
	}
}
