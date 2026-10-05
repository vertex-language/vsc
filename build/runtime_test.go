package build_test

import (
	"fmt"
	"io/fs"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/vertex-language/ir"
	"github.com/vertex-language/vcx"

	"github.com/vertex-language/vsc/build"
	"github.com/vertex-language/vsc/stdlib"
)

// The runtime's own tests: C++ programs in stdlib/tests that include the
// runtime and exercise it directly, compiled by vcx and linked by this
// package, with no toolchain from the platform involved.

// runtimeProgram compiles one of stdlib/tests' drivers together with the
// runtime and generated headers, links it, runs it, and answers its
// standard output.
func runtimeProgram(t *testing.T, driver string, generated map[string]string) string {
	t.Helper()
	target, ok := build.Host()
	if !ok {
		t.Skip("no backend for this machine")
	}
	name := hostTargetName(target)
	unit, ok := stdlib.RuntimeUnit(name)
	if !ok {
		t.Skipf("no runtime platform layer for %s", name)
	}
	src, err := os.ReadFile(filepath.Join("..", "stdlib", "tests", driver))
	if err != nil {
		t.Fatal(err)
	}
	extra := fstest.MapFS{
		// The driver says platform.h and means this target's layer.
		"platform.h": {Data: []byte("#include \"platform/" + strings.TrimSuffix(unit, ".cpp") + ".h\"\n")},
	}
	for file, text := range generated {
		extra[file] = &fstest.MapFile{Data: []byte(text)}
	}
	runtime := stdlib.Runtime()
	include, err := fs.Sub(runtime, "include")
	if err != nil {
		t.Fatal(err)
	}
	c := &vcx.Compiler{
		Target:       name,
		Std:          vcx.Cxx23,
		Freestanding: true,
		IncludeFS: []vcx.SystemInclude{
			{Name: "<vertex>", FS: include},
			{Name: "<runtime>", FS: runtime},
			{Name: "<generated>", FS: extra},
		},
	}
	m, diags, err := c.IR(vcx.Text(driver, src))
	if err == nil && vcx.HasErrors(diags) {
		err = &vcx.DiagnosticError{Diagnostics: diags}
	}
	if err != nil {
		t.Fatalf("vcx: %v", err)
	}
	// The runtime's assembly is carried by its own object (see
	// build.Runtime); a driver compiling the runtime's sources carries
	// it too.
	if text, ok := stdlib.TaskAsm(name); ok {
		m.Asm(text)
	}
	obj, err := build.Object(m, build.Options{})
	if err != nil {
		t.Fatal(err)
	}
	exe, err := build.Executable([]build.Input{{Name: driver + ".o", Data: obj}},
		build.LinkOptions{Target: target, NoRuntime: true})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "driver")
	if err := os.WriteFile(path, exe, 0o755); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(path).Output()
	if err != nil {
		t.Fatalf("%s: %v\n%s", driver, err, out)
	}
	return string(out)
}

func hostTargetName(target ir.Target) string {
	switch target.Use() {
	case "aarch64/macos":
		return "aarch64-macos"
	case "x86_64/windows":
		return "x86_64-windows"
	}
	return ""
}

// TestRuntimeUnicodeConformance runs every case of the Unicode
// Consortium's GraphemeBreakTest and NormalizationTest through the
// runtime's grapheme counting and canonical comparison. They are the
// definition String.count and == are held to.
func TestRuntimeUnicodeConformance(t *testing.T) {
	var b strings.Builder
	b.WriteString("struct GraphemeCase { const vertex::u8* bytes; vertex::usize count; vertex::usize clusters; vertex::u32 line; };\n")
	b.WriteString("struct NormalizationCase { const vertex::u8* source; vertex::usize sourceCount; " +
		"const vertex::u8* composed; vertex::usize composedCount; const vertex::u8* decomposed; " +
		"vertex::usize decomposedCount; const vertex::u32* nfc; vertex::usize nfcCount; vertex::u32 line; };\n")

	var cases []string
	eachLine(t, "GraphemeBreakTest.txt", func(line int, fields []string) {
		var cps []rune
		clusters := -1
		for _, f := range strings.Fields(fields[0]) {
			switch f {
			case "÷":
				clusters++
			case "×":
			default:
				cps = append(cps, hexRune(t, f))
			}
		}
		id := fmt.Sprintf("g%d", len(cases))
		fmt.Fprintf(&b, "static const vertex::u8 %s[] = %s;\n", id, byteList(string(cps)))
		cases = append(cases, fmt.Sprintf("{%s, %d, %d, %d}", id, len(string(cps)), clusters, line))
	})
	fmt.Fprintf(&b, "static const GraphemeCase graphemeCases[] = {%s};\n", strings.Join(cases, ",\n"))

	cases = cases[:0]
	eachLine(t, "NormalizationTest.txt", func(line int, fields []string) {
		if strings.HasPrefix(fields[0], "@") {
			return
		}
		cols := make([]string, 3)
		for i := range cols {
			var s []rune
			for _, h := range strings.Fields(fields[i]) {
				s = append(s, hexRune(t, h))
			}
			cols[i] = string(s)
		}
		id := fmt.Sprintf("n%d", len(cases))
		var nfc []string
		for _, r := range cols[1] {
			nfc = append(nfc, fmt.Sprint(uint32(r)))
		}
		fmt.Fprintf(&b, "static const vertex::u8 %s_s[] = %s;\nstatic const vertex::u8 %s_c[] = %s;\n"+
			"static const vertex::u8 %s_d[] = %s;\nstatic const vertex::u32 %s_n[] = {%s};\n",
			id, byteList(cols[0]), id, byteList(cols[1]), id, byteList(cols[2]), id, strings.Join(nfc, ","))
		cases = append(cases, fmt.Sprintf("{%s_s, %d, %s_c, %d, %s_d, %d, %s_n, %d, %d}",
			id, len(cols[0]), id, len(cols[1]), id, len(cols[2]), id, len([]rune(cols[1])), line))
	})
	fmt.Fprintf(&b, "static const NormalizationCase normalizationCases[] = {%s};\n", strings.Join(cases, ",\n"))

	if out := runtimeProgram(t, "unicode.cpp", map[string]string{"cases.h": b.String()}); out != "" {
		t.Errorf("conformance failures:\n%s", out)
	}
}

// TestRuntimeFloatDescriptionsMatchSwift puts forty thousand doubles and
// floats -- edge cases, then random bit patterns and random decimals --
// through the runtime's description and swiftc's, and compares.
func TestRuntimeFloatDescriptionsMatchSwift(t *testing.T) {
	swiftc, err := exec.LookPath("swiftc")
	if err != nil {
		t.Skip("no swiftc on PATH; there is no oracle to compare against")
	}
	rng := rand.New(rand.NewSource(7))
	doubles := []float64{0, math.Copysign(0, -1), 1, 1.5, 0.1, 0.3, 100, 1e15, 1e16, 1 << 53,
		1<<53 + 2, 1e-4, 1e-5, 5e-324, math.MaxFloat64, math.Inf(1), math.Inf(-1),
		2.2250738585072014e-308, 2.225073858507201e-308, 4.35, 2.0 / 3, 9.999999999999999e22, 1e23}
	var dbits []uint64
	for _, d := range doubles {
		dbits = append(dbits, math.Float64bits(d))
	}
	for i := 0; i < 20000; i++ {
		var bits uint64
		switch i % 3 {
		case 0:
			bits = rng.Uint64()
		case 1:
			bits = math.Float64bits((rng.Float64() - 0.5) * 2e6)
		default:
			scale := math.Pow(10, float64(rng.Intn(7)))
			bits = math.Float64bits(math.Round(rng.Float64()*1000*scale) / scale)
		}
		if !math.IsNaN(math.Float64frombits(bits)) {
			dbits = append(dbits, bits)
		}
	}
	var fbits []uint32
	for _, f := range []float32{0, 1, 1.5, 0.1, 16777216, 1e7, 1e8, math.MaxFloat32, 1.4e-45, 1e-4, 1e-5} {
		fbits = append(fbits, math.Float32bits(f))
	}
	for i := 0; i < 20000; i++ {
		if bits := rng.Uint32(); !math.IsNaN(float64(math.Float32frombits(bits))) {
			fbits = append(fbits, bits)
		}
	}

	var cpp, swift strings.Builder
	cpp.WriteString("static const vertex::u64 doubles[] = {")
	swift.WriteString("let doubles: [UInt64] = [")
	for i, b := range dbits {
		if i > 0 {
			cpp.WriteByte(',')
			swift.WriteByte(',')
		}
		fmt.Fprintf(&cpp, "0x%XULL", b)
		fmt.Fprintf(&swift, "0x%X", b)
	}
	cpp.WriteString("};\nstatic const vertex::u32 floats[] = {")
	swift.WriteString("]\nlet floats: [UInt32] = [")
	for i, b := range fbits {
		if i > 0 {
			cpp.WriteByte(',')
			swift.WriteByte(',')
		}
		fmt.Fprintf(&cpp, "0x%XU", b)
		fmt.Fprintf(&swift, "0x%X", b)
	}
	cpp.WriteString("};\n")
	swift.WriteString("]\nvar out = \"\"\n" +
		"for b in doubles { out += Double(bitPattern: b).description; out += \"\\n\" }\n" +
		"for b in floats { out += Float(bitPattern: b).description; out += \"\\n\" }\n" +
		"print(out, terminator: \"\")\n")

	dir := t.TempDir()
	main := filepath.Join(dir, "main.swift")
	if err := os.WriteFile(main, []byte(swift.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	oracle := filepath.Join(dir, "oracle")
	if out, err := exec.Command(swiftc, "-O", "-o", oracle, main).CombinedOutput(); err != nil {
		t.Fatalf("swiftc: %v\n%s", err, out)
	}
	want, err := exec.Command(oracle).Output()
	if err != nil {
		t.Fatal(err)
	}
	got := runtimeProgram(t, "floats.cpp", map[string]string{"values.h": cpp.String()})

	wantLines, gotLines := strings.Split(string(want), "\n"), strings.Split(got, "\n")
	if len(wantLines) != len(gotLines) {
		t.Fatalf("%d descriptions from the runtime, %d from swiftc", len(gotLines), len(wantLines))
	}
	bad := 0
	for i := range wantLines {
		if wantLines[i] != gotLines[i] && bad < 20 {
			bad++
			t.Errorf("value %d: runtime %q, swiftc %q", i, gotLines[i], wantLines[i])
		}
	}
}

func eachLine(t *testing.T, file string, fn func(line int, fields []string)) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "stdlib", "tests", "testdata", file))
	if err != nil {
		t.Fatal(err)
	}
	for i, line := range strings.Split(string(data), "\n") {
		if j := strings.IndexByte(line, '#'); j >= 0 {
			line = line[:j]
		}
		if strings.TrimSpace(line) == "" {
			continue
		}
		fn(i+1, strings.Split(line, ";"))
	}
}

func hexRune(t *testing.T, s string) rune {
	t.Helper()
	var r rune
	if _, err := fmt.Sscanf(s, "%X", &r); err != nil {
		t.Fatalf("bad code point %q", s)
	}
	return r
}

func byteList(s string) string {
	if s == "" {
		return "{0}"
	}
	parts := make([]string, len(s))
	for i := 0; i < len(s); i++ {
		parts[i] = fmt.Sprint(s[i])
	}
	return "{" + strings.Join(parts, ",") + "}"
}

// TestRuntimeDebugDescriptionsMatchSwift puts strings through the
// runtime's debug description and swiftc's String.debugDescription: the
// escapes, and the rule that a scalar which would fuse into one
// Character with a quote or an escape is escaped itself.
func TestRuntimeDebugDescriptionsMatchSwift(t *testing.T) {
	swiftc, err := exec.LookPath("swiftc")
	if err != nil {
		t.Skip("no swiftc on PATH; there is no oracle to compare against")
	}
	rng := rand.New(rand.NewSource(11))
	interesting := []rune{0, 1, '\t', '\n', '\r', '"', '\'', '\\', 0x7F, 0x85, 0xAD, 0x301, 0x600,
		0x903, 0x1100, 0x1161, 0x11A8, 0xAC00, 0x200D, 0x2028, 0xFEFF, 0xE000, 0x1F1FA,
		0x1F1F8, 0x1F600, 0x1F3FB, 0xE0100, 'a', 'e', ' ', 0x915, 0x94D}
	samples := []string{"", "plain", "a\"b\n\tc é", "\u0301", "x\u0600", "\u0600x", "\\\u0301",
		"\t\u0301e\u0301\u200d", "\U0001F1FA\U0001F1F8", "\u1100\u1161\u11A8"}
	for i := 0; i < 3000; i++ {
		n := 1 + rng.Intn(5)
		var rs []rune
		for j := 0; j < n; j++ {
			rs = append(rs, interesting[rng.Intn(len(interesting))])
		}
		samples = append(samples, string(rs))
	}

	var cpp, swift strings.Builder
	cpp.WriteString("struct Bytes { const vertex::u8* bytes; vertex::u64 count; };\n")
	var entries []string
	var flat, lengths []string
	for i, s := range samples {
		fmt.Fprintf(&cpp, "static const vertex::u8 s%d[] = %s;\n", i, byteList(s))
		entries = append(entries, fmt.Sprintf("{s%d, %d}", i, len(s)))
		for j := 0; j < len(s); j++ {
			flat = append(flat, fmt.Sprint(s[j]))
		}
		lengths = append(lengths, fmt.Sprint(len(s)))
	}
	// Flat arrays of integers, which swiftc type-checks at once, rather
	// than an array of strings, which it takes most of a minute over.
	fmt.Fprintf(&swift, "let bytes: [UInt8] = [%s]\nlet lengths: [Int] = [%s]\n",
		strings.Join(flat, ","), strings.Join(lengths, ","))
	fmt.Fprintf(&cpp, "static const Bytes strings[] = {%s};\n", strings.Join(entries, ","))
	swift.WriteString("var out = \"\"\nvar at = 0\nfor n in lengths {\n" +
		"  let s = String(decoding: bytes[at..<at+n], as: UTF8.self)\n  at += n\n" +
		"  out += s.debugDescription; out += \"\\n\"\n}\nprint(out, terminator: \"\")\n")

	dir := t.TempDir()
	main := filepath.Join(dir, "main.swift")
	if err := os.WriteFile(main, []byte(swift.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	oracle := filepath.Join(dir, "oracle")
	if out, err := exec.Command(swiftc, "-o", oracle, main).CombinedOutput(); err != nil {
		t.Fatalf("swiftc: %v\n%s", err, out)
	}
	want, err := exec.Command(oracle).Output()
	if err != nil {
		t.Fatal(err)
	}
	got := runtimeProgram(t, "escapes.cpp", map[string]string{"strings.h": cpp.String()})
	wantLines, gotLines := strings.Split(string(want), "\n"), strings.Split(got, "\n")
	if len(wantLines) != len(gotLines) {
		t.Fatalf("%d descriptions from the runtime, %d from swiftc", len(gotLines), len(wantLines))
	}
	bad := 0
	for i := range wantLines {
		if wantLines[i] != gotLines[i] && bad < 20 {
			bad++
			t.Errorf("%q: runtime %s, swiftc %s", samples[i], gotLines[i], wantLines[i])
		}
	}
}

// TestRuntimeCollections runs the runtime's Array mutation and hash table
// through stdlib/tests/collections.cpp: copy on write, growth, removal by
// backward shift, and canonically equal String keys.
func TestRuntimeCollections(t *testing.T) {
	if out := runtimeProgram(t, "collections.cpp", nil); out != "" {
		t.Errorf("collection failures:\n%s", out)
	}
}

// TestRuntimeTaskAllocator: the bump pointer an async function's frames
// come from, which everything about the async ABI rests on. See
// docs/vertex_swift_async.md.
func TestRuntimeTaskAllocator(t *testing.T) {
	if out := runtimeProgram(t, "tasks.cpp", nil); out != "" {
		t.Errorf("task allocator failures:\n%s", out)
	}
}
