package build_test

// Swift's async calling convention, across a real link.
//
// An async function is handed its context in the async register, returns
// nothing, and hands its result to the continuation the context names:
//
//	+0  parent        the caller's context
//	+8  resumeParent  where to go when this function returns
//
// So the other end of the call is written in C here, and it is the half
// that matters: it builds a context, calls the Vertex function with it,
// and records what arrives at the continuation. If the compiler returned
// a value in X0 instead, nothing would arrive and nothing would crash --
// which is why this is a run test.
//
// See docs/vertex_swift_async.md and docs/async_refactor_plan.md.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vertex-language/ir"

	vsc "github.com/vertex-language/vsc"
	"github.com/vertex-language/vsc/analyzer"
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/build"
	"github.com/vertex-language/vsc/internal/sil/gen"
	"github.com/vertex-language/vsc/internal/sil/pass"
	"github.com/vertex-language/vsc/lower"
	"github.com/vertex-language/vsc/parser"
	"github.com/vertex-language/vsc/token"
)

// asyncObject compiles src with the async ABI turned on and returns its
// object file.
func asyncObject(t *testing.T, target ir.Target, src string) []byte {
	t.Helper()
	f := token.NewFile("a.vs", []byte(src))
	file, diags := parser.ParseFile(f, 0)
	for _, d := range diags {
		t.Fatalf("parse: %s", d.Print(f))
	}
	info, checks := analyzer.Check([]*ast.File{file})
	for _, d := range checks {
		t.Fatalf("check: %s", d.Print(f))
	}
	m, gd := gen.Files("lib", []*ast.File{file}, info)
	for _, d := range gd {
		t.Fatalf("gen: %s", d.Print(f))
	}
	if err := pass.Mandatory(m); err != nil {
		t.Fatalf("passes: %v", err)
	}
	if err := pass.LowerOwnership(m); err != nil {
		t.Fatalf("ownership: %v", err)
	}
	vir, err := lower.Module(m, target, lower.Options{
		SymbolPrefix: vsc.SymbolPrefix(target),
	})
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	obj, err := build.Object(vir, build.Options{})
	if err != nil {
		t.Fatal(err)
	}
	return obj
}

// runWithDriver links an object against a C driver and runs it.
func runWithDriver(t *testing.T, target ir.Target, obj []byte, driver string) string {
	t.Helper()
	dir := t.TempDir()
	objPath := filepath.Join(dir, "lib.o")
	if err := os.WriteFile(objPath, obj, 0o644); err != nil {
		t.Fatal(err)
	}
	cPath := filepath.Join(dir, "driver.c")
	if err := os.WriteFile(cPath, []byte(driver), 0o644); err != nil {
		t.Fatal(err)
	}
	cc, err := exec.LookPath("cc")
	if err != nil {
		t.Skip("no cc to write the other end of the call with")
	}
	// The runtime's symbols a driven object may name, stood in for: where
	// a failed overflow check goes, and Error's descriptor, which a
	// conformance to it points at.
	stubPath := filepath.Join(dir, "runtime_stubs.c")
	const stubs = `
#include <stdio.h>
#include <stdlib.h>
void vertex_fatal(const char *message) { fprintf(stderr, "Fatal error: %s\n", message); abort(); }
const int vertex_error_protocol[6] __asm__("_$ss5ErrorMp") = {3, 0, 0, 0, 0, 0};
`
	if err := os.WriteFile(stubPath, []byte(stubs), 0o644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "prog")
	out, err := exec.Command(cc, "-o", bin, cPath, objPath, stubPath).CombinedOutput()
	if err != nil {
		t.Fatalf("link: %v\n%s", err, out)
	}
	got, err := exec.Command(bin).CombinedOutput()
	if err != nil {
		t.Fatalf("run: %v\n%s", err, got)
	}
	return string(got)
}

// TestAsyncReturnsThroughItsContinuation: an async function's result
// reaches the continuation its context names, rather than coming back in
// a return register.
func TestAsyncReturnsThroughItsContinuation(t *testing.T) {
	target, ok := build.Host()
	if !ok {
		t.Skip("no backend")
	}
	if !strings.HasSuffix(target.Use(), "/macos") {
		t.Skip("the driver is written for this host's C compiler")
	}

	obj := asyncObject(t, target, `
public func twice(_ n: int) async -> int { return n * 2 }
`)

	got := runWithDriver(t, target, obj, `
#include <stdio.h>
#include <stdint.h>

// The context an async function is handed: the caller's, and where to go
// when it returns. Its frame would follow at +16.
struct Ctx {
    struct Ctx *parent;
    void (*resumeParent)(void);
};

static volatile long answer = -1;
static volatile int  arrived = 0;

// The continuation. It is entered with the context in the async register
// and the result as an ordinary argument, so it is declared to take the
// context where the ABI puts it -- which for this test only needs the
// second argument to be right.
__asm__(
"   .text\n"
"   .globl _resume\n"
"   .p2align 2\n"
"_resume:\n"
"   adrp x9, _answer@PAGE\n"
"   add  x9, x9, _answer@PAGEOFF\n"
"   str  x0, [x9]\n"            // the result, in the first argument register
"   adrp x9, _arrived@PAGE\n"
"   add  x9, x9, _arrived@PAGEOFF\n"
"   mov  w10, #1\n"
"   str  w10, [x9]\n"
"   ret\n"
);
void resume(void);

// The Vertex function, by its mangled name: twice(int) async -> int.
extern void lib_twice(void) __asm__("_$s3lib5twiceyS2iYaF");

int main(void) {
    struct Ctx ctx = { 0, resume };
    // Call it with the context in X22 and the argument in X0.
    __asm__ volatile(
        "mov x22, %0\n"
        "mov x0, %1\n"
        "bl  _$s3lib5twiceyS2iYaF\n"
        :
        : "r"(&ctx), "r"(21L)
        : "x0", "x1", "x2", "x3", "x4", "x5", "x6", "x7",
          "x8", "x9", "x10", "x11", "x12", "x13", "x14", "x15",
          "x16", "x17", "x22", "x30", "memory"
    );
    printf("%s arrived=%d answer=%ld\n",
           (arrived && answer == 42) ? "ok" : "WRONG", arrived, answer);
    return 0;
}
`)
	if !strings.HasPrefix(got, "ok ") {
		t.Errorf("driver printed %q", got)
	}
}

// TestAsyncContextIsThreadedThrough: an async function that calls
// another one hands it a context, and the answer comes back through the
// chain. Here the inner call is the Vertex function and the outer
// context is the driver's, so what is under test is that the callee
// reads its context from the async register and nothing else.
func TestAsyncContextIsThreadedThrough(t *testing.T) {
	target, ok := build.Host()
	if !ok {
		t.Skip("no backend")
	}
	if !strings.HasSuffix(target.Use(), "/macos") {
		t.Skip("the driver is written for this host's C compiler")
	}

	// Two ordinary arguments as well as the context, so that a context
	// placed in the argument sequence by mistake would shift them.
	obj := asyncObject(t, target, `
public func combine(_ a: int, _ b: int) async -> int { return a * 100 + b }
`)

	got := runWithDriver(t, target, obj, `
#include <stdio.h>

struct Ctx {
    struct Ctx *parent;
    void (*resumeParent)(void);
};

static volatile long answer = -1;
static volatile long sawCtx = 0;

// The continuation records the result and the context it was handed, so
// that a context which did not survive the call is visible too.
__asm__(
"   .text\n"
"   .globl _resume2\n"
"   .p2align 2\n"
"_resume2:\n"
"   adrp x9, _answer@PAGE\n"
"   add  x9, x9, _answer@PAGEOFF\n"
"   str  x0, [x9]\n"
"   adrp x9, _sawCtx@PAGE\n"
"   add  x9, x9, _sawCtx@PAGEOFF\n"
"   str  x22, [x9]\n"           // the context, from the async register
"   ret\n"
);
void resume2(void);

int main(void) {
    struct Ctx ctx = { 0, resume2 };
    __asm__ volatile(
        "mov x22, %0\n"
        "mov x0, %1\n"
        "mov x1, %2\n"
        "bl  _$s3lib7combineyS2i_SitYaF\n"
        :
        : "r"(&ctx), "r"(7L), "r"(9L)
        : "x0", "x1", "x2", "x3", "x4", "x5", "x6", "x7",
          "x8", "x9", "x10", "x11", "x12", "x13", "x14", "x15",
          "x16", "x17", "x22", "x30", "memory"
    );
    int okCtx = (sawCtx == (long)&ctx);
    printf("%s answer=%ld ctx=%d\n",
           (answer == 709 && okCtx) ? "ok" : "WRONG", answer, okCtx);
    return 0;
}
`)
	if !strings.HasPrefix(got, "ok ") {
		t.Errorf("driver printed %q", got)
	}
}

// TestAsyncSuspensionRunsThrough is the split itself: a function that
// awaits another one, cut into a prologue, a body behind a dispatch, and
// a stub the callee returns into.
//
// The driver supplies the task allocator, so what is under test is the
// compiler's half of the protocol and nothing of the runtime's: whether
// the caller allocates the callee's context and points it back at
// itself, whether the callee returns into the stub, and whether the
// value computed before the await is still there afterwards.
func TestAsyncSuspensionRunsThrough(t *testing.T) {
	target, ok := build.Host()
	if !ok {
		t.Skip("no backend")
	}
	if !strings.HasSuffix(target.Use(), "/macos") {
		t.Skip("the driver is written for this host's C compiler")
	}

	// x is computed before the await and used after it, so it has to
	// survive in the frame. y comes back from the call.
	obj := asyncObject(t, target, `
func inner(_ n: int) async -> int { return n + 1 }
public func outer(_ a: int) async -> int {
    let x = a * 10
    let y = await inner(a)
    return x + y
}
`)

	got := runWithDriver(t, target, obj, `
#include <stdio.h>
#include <stdint.h>

struct Ctx {
    struct Ctx *parent;
    void (*resumeParent)(void);
    char frame[512];
};

// The task allocator, which the runtime normally provides: a bump
// pointer with stack discipline, which is all the protocol asks for.
static char pool[1 << 16];
static char *bump = pool;
void *vertex_task_alloc(unsigned long n) {
    void *p = bump;
    bump += (n + 15) & ~15UL;
    return p;
}
void vertex_task_dealloc(void *p) { if (p) bump = (char *)p; }

static volatile long answer = -1;
static volatile int  arrived = 0;

__asm__(
"   .text\n"
"   .globl _done\n"
"   .p2align 2\n"
"_done:\n"
"   adrp x9, _answer@PAGE\n"
"   add  x9, x9, _answer@PAGEOFF\n"
"   str  x0, [x9]\n"
"   adrp x9, _arrived@PAGE\n"
"   add  x9, x9, _arrived@PAGEOFF\n"
"   mov  w10, #1\n"
"   str  w10, [x9]\n"
"   ret\n"
);
void done(void);

int main(void) {
    static struct Ctx ctx;
    ctx.parent = 0;
    ctx.resumeParent = done;
    __asm__ volatile(
        "mov x22, %0\n"
        "mov x0, %1\n"
        "bl  _$s3lib5outeryS2iYaF\n"
        :
        : "r"(&ctx), "r"(5L)
        : "x0", "x1", "x2", "x3", "x4", "x5", "x6", "x7",
          "x8", "x9", "x10", "x11", "x12", "x13", "x14", "x15",
          "x16", "x17", "x22", "x30", "memory"
    );
    // 5*10 + (5+1) = 56
    printf("%s arrived=%d answer=%ld\n",
           (arrived && answer == 56) ? "ok" : "WRONG", arrived, answer);
    return 0;
}
`)
	if !strings.HasPrefix(got, "ok ") {
		t.Errorf("driver printed %q", got)
	}
}

// TestAsyncTwoSuspensionsAndALoop: the two shapes a split has to get
// right beyond the simple case.
//
// Two awaits in a row means two stubs and a dispatch that tells them
// apart. A loop with an await in it means the block after the suspension
// branches back to the block before it — which is why the body is one
// function behind a dispatch rather than a chain of funclets, and is the
// case a chain cannot express at all.
func TestAsyncTwoSuspensionsAndALoop(t *testing.T) {
	target, ok := build.Host()
	if !ok {
		t.Skip("no backend")
	}
	if !strings.HasSuffix(target.Use(), "/macos") {
		t.Skip("the driver is written for this host's C compiler")
	}

	obj := asyncObject(t, target, `
func inner(_ n: int) async -> int { return n + 1 }

// Two in a row, with a value carried past both.
public func twice(_ a: int) async -> int {
    let base = a * 100
    let p = await inner(a)
    let q = await inner(p)
    return base + p + q
}

// An await inside a loop: the total is carried round, and the block the
// loop comes back to is before the suspension.
public func summing(_ n: int) async -> int {
    var total = 0
    var i = 0
    while i < n {
        total = total + (await inner(i))
        i = i + 1
    }
    return total
}
`)

	got := runWithDriver(t, target, obj, `
#include <stdio.h>

struct Ctx {
    struct Ctx *parent;
    void (*resumeParent)(void);
    char frame[1024];
};

static char pool[1 << 20];
static char *bump = pool;
void *vertex_task_alloc(unsigned long n) {
    void *p = bump;
    bump += (n + 15) & ~15UL;
    return p;
}
void vertex_task_dealloc(void *p) { if (p) bump = (char *)p; }

static volatile long answer = -1;
static volatile int  arrived = 0;

__asm__(
"   .text\n"
"   .globl _done\n"
"   .p2align 2\n"
"_done:\n"
"   adrp x9, _answer@PAGE\n"
"   add  x9, x9, _answer@PAGEOFF\n"
"   str  x0, [x9]\n"
"   adrp x9, _arrived@PAGE\n"
"   add  x9, x9, _arrived@PAGEOFF\n"
"   mov  w10, #1\n"
"   str  w10, [x9]\n"
"   ret\n"
);
void done(void);

#define CLOBBERS "x0","x1","x2","x3","x4","x5","x6","x7","x8","x9","x10",\
                 "x11","x12","x13","x14","x15","x16","x17","x22","x30","memory"

static long call1(void (*fn)(void), long a) {
    static struct Ctx ctx;
    ctx.parent = 0;
    ctx.resumeParent = done;
    bump = pool;
    arrived = 0;
    answer = -1;
    __asm__ volatile(
        "mov x22, %0\n"
        "mov x0, %1\n"
        "blr %2\n"
        :
        : "r"(&ctx), "r"(a), "r"(fn)
        : CLOBBERS
    );
    return arrived ? answer : -1;
}

extern void twice_(void)   __asm__("_$s3lib5twiceyS2iYaF");
extern void summing_(void) __asm__("_$s3lib7summingyS2iYaF");

int main(void) {
    // base 300, p = 4, q = 5  ->  309
    long a = call1(twice_, 3);
    // 1 + 2 + 3 + 4 = 10
    long b = call1(summing_, 4);
    printf("%s twice=%ld summing=%ld\n",
           (a == 309 && b == 10) ? "ok" : "WRONG", a, b);
    return 0;
}
`)
	if !strings.HasPrefix(got, "ok ") {
		t.Errorf("driver printed %q", got)
	}
}

// TestAsyncThrowsThroughTheSelfRegister: a throwing async function does
// not bring its error back in the error register. It hands it to the
// continuation, last, in the self register, and null there means nothing
// was thrown.
//
// This is measured from swiftc rather than chosen. Its failing path is
//
//	musttail call swifttailcc void %7(ptr swiftasync %0, i64 undef, ptr swiftself %5)
//
// and its ordinary one passes null in the same place. A compiler that
// used the error register instead would link and would answer with
// whatever was in X20, so this has to be a run test.
func TestAsyncThrowsThroughTheSelfRegister(t *testing.T) {
	target, ok := build.Host()
	if !ok {
		t.Skip("no backend")
	}
	if !strings.HasSuffix(target.Use(), "/macos") {
		t.Skip("the driver is written for this host's C compiler")
	}

	obj := asyncObject(t, target, `
struct Bad: Error { var code: int }

// Fails for a negative argument and answers for the rest, so one
// function covers both paths through the same continuation.
public func checked(_ n: int) async throws -> int {
    if n < 0 { throw Bad(code: n) }
    return n + 1
}
`)

	got := runWithDriver(t, target, obj, `
#include <stdio.h>


struct Ctx {
    struct Ctx *parent;
    void (*resumeParent)(void);
    char frame[256];
};

static char pool[1 << 16];
static char *bump = pool;
void *vertex_task_alloc(unsigned long n) {
    void *p = bump;
    bump += (n + 15) & ~15UL;
    return p;
}
void vertex_task_dealloc(void *p) { if (p) bump = (char *)p; }

// What throwing reaches for, stubbed: this end of the call cares that a
// box arrived and not what is in it.
static char boxed[64];
void *vertex_error_box(void *existential) { (void)existential; return boxed; }
long vertex_metadata_Int;

static volatile long answer  = -1;
static volatile long thrown  = -1;
static volatile int  arrived = 0;

// The continuation: the result in the first argument register, the error
// in the self register.
__asm__(
"   .text\n"
"   .globl _resume\n"
"   .p2align 2\n"
"_resume:\n"
"   adrp x9, _answer@PAGE\n"
"   add  x9, x9, _answer@PAGEOFF\n"
"   str  x0, [x9]\n"
"   adrp x9, _thrown@PAGE\n"
"   add  x9, x9, _thrown@PAGEOFF\n"
"   str  x20, [x9]\n"
"   adrp x9, _arrived@PAGE\n"
"   add  x9, x9, _arrived@PAGEOFF\n"
"   mov  w10, #1\n"
"   str  w10, [x9]\n"
"   ret\n"
);
void resume(void);

#define CLOBBERS "x0","x1","x2","x3","x4","x5","x6","x7","x8","x9","x10",\
                 "x11","x12","x13","x14","x15","x16","x17","x20","x22","x30","memory"

static void call1(long a) {
    static struct Ctx ctx;
    ctx.parent = 0;
    ctx.resumeParent = resume;
    bump = pool;
    arrived = 0;
    answer = -1;
    thrown = -1;
    __asm__ volatile(
        "mov x22, %0\n"
        "mov x0, %1\n"
        "bl  _$s3lib7checkedyS2iYaKF\n"
        :
        : "r"(&ctx), "r"(a)
        : CLOBBERS
    );
}

int main(void) {
    call1(7);
    int okGood = arrived && answer == 8 && thrown == 0;
    long good = answer;
    call1(-3);
    // The box is a reference to something; which object it is belongs to
    // the runtime, and all this end of the call can say is that one
    // arrived where nothing arrived before.
    int okBad = arrived && thrown != 0;
    printf("%s good=%ld threw=%d answer=%ld err=%ld\n",
           (okGood && okBad) ? "ok" : "WRONG", good, okBad, answer, thrown);
    return 0;
}
`)
	if !strings.HasPrefix(got, "ok ") {
		t.Errorf("driver printed %q", got)
	}
}
