package analyzer

import (
	"strings"
	"testing"

	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/parser"
	"github.com/vertex-language/vsc/token"
	"github.com/vertex-language/vsc/types"
)

func parseSnippet(t *testing.T, src string) (*ast.File, *token.File) {
	t.Helper()
	f := token.NewFile("snippet.swift", []byte(src))
	file, diags := parser.ParseFile(f, 0)
	if len(diags) != 0 {
		for _, d := range diags {
			t.Errorf("parser diag: %s", d.Print(f))
		}
		t.Fatalf("parse failed for snippet: %s", src)
	}
	return file, f
}

func TestPrecedenceGraph(t *testing.T) {
	pg := NewPrecedenceGraph()

	// Multiplication > Addition
	if !pg.HigherThan("MultiplicationPrecedence", "AdditionPrecedence") {
		t.Errorf("MultiplicationPrecedence should be higher than AdditionPrecedence")
	}
	// Addition > Comparison
	if !pg.HigherThan("AdditionPrecedence", "ComparisonPrecedence") {
		t.Errorf("AdditionPrecedence should be higher than ComparisonPrecedence")
	}
	// Comparison > LogicalConjunction
	if !pg.HigherThan("ComparisonPrecedence", "LogicalConjunctionPrecedence") {
		t.Errorf("ComparisonPrecedence should be higher than LogicalConjunctionPrecedence")
	}
	// LogicalConjunction > LogicalDisjunction
	if !pg.HigherThan("LogicalConjunctionPrecedence", "LogicalDisjunctionPrecedence") {
		t.Errorf("LogicalConjunctionPrecedence should be higher than LogicalDisjunctionPrecedence")
	}
	// LogicalDisjunction > Ternary
	if !pg.HigherThan("LogicalDisjunctionPrecedence", "TernaryPrecedence") {
		t.Errorf("LogicalDisjunctionPrecedence should be higher than TernaryPrecedence")
	}
	// Ternary > Assignment
	if !pg.HigherThan("TernaryPrecedence", "AssignmentPrecedence") {
		t.Errorf("TernaryPrecedence should be higher than AssignmentPrecedence")
	}

	// Transitive: Multiplication > Assignment
	if !pg.HigherThan("MultiplicationPrecedence", "AssignmentPrecedence") {
		t.Errorf("MultiplicationPrecedence should transitively be higher than AssignmentPrecedence")
	}
	// Incomparable / equal
	if pg.HigherThan("AdditionPrecedence", "AdditionPrecedence") {
		t.Errorf("Precedence group cannot be higher than itself")
	}

	// Operator group lookups
	if pg.OperatorGroup("+").Name != "AdditionPrecedence" {
		t.Errorf("expected '+' to be AdditionPrecedence")
	}
	if pg.OperatorGroup("*").Name != "MultiplicationPrecedence" {
		t.Errorf("expected '*' to be MultiplicationPrecedence")
	}
}

func TestFoldSequence(t *testing.T) {
	pg := NewPrecedenceGraph()

	// 1 + 2 * 3 -> 1 + (2 * 3)
	file, f := parseSnippet(t, "let x = 1 + 2 * 3")
	var seq *ast.SequenceExpr
	ast.Inspect(file, func(n ast.Node) bool {
		if s, ok := n.(*ast.SequenceExpr); ok {
			seq = s
			return false
		}
		return true
	})
	if seq == nil {
		t.Fatalf("SequenceExpr not found")
	}

	folded, err := FoldSequence(f, seq, pg)
	if err != nil {
		t.Fatalf("FoldSequence failed: %v", err)
	}

	bin, ok := folded.(*ast.BinaryExpr)
	if !ok {
		t.Fatalf("expected root to be BinaryExpr, got %T", folded)
	}
	opName := string(f.Slice(bin.Op.Lo, bin.Op.Hi))
	if opName != "+" {
		t.Errorf("expected root op '+', got %q", opName)
	}
	rhsBin, ok := bin.Y.(*ast.BinaryExpr)
	if !ok {
		t.Fatalf("expected rhs to be BinaryExpr, got %T", bin.Y)
	}
	rhsOp := string(f.Slice(rhsBin.Op.Lo, rhsBin.Op.Hi))
	if rhsOp != "*" {
		t.Errorf("expected rhs op '*', got %q", rhsOp)
	}
}

func TestScopesAndSymbols(t *testing.T) {
	uScope := NewScope(nil, token.NoPos, token.NoPos)
	pkgScope := NewScope(uScope, token.NoPos, token.NoPos)

	v1 := NewVar("x", types.Typ[types.Int], token.NoPos, true, types.DefaultOwnership)
	if old := pkgScope.Insert(v1); old != nil {
		t.Errorf("insert failed")
	}
	if pkgScope.Lookup("x") != v1 {
		t.Errorf("lookup failed")
	}

	// Duplicate insert
	v2 := NewVar("x", types.Typ[types.String], token.NoPos, false, types.DefaultOwnership)
	if old := pkgScope.Insert(v2); old != v1 {
		t.Errorf("expected duplicate insert to return v1")
	}

	// Child scope shadowing
	localScope := NewScope(pkgScope, token.NoPos, token.NoPos)
	v3 := NewVar("x", types.Typ[types.Bool], token.NoPos, false, types.DefaultOwnership)
	localScope.Insert(v3)

	if localScope.Lookup("x") != v3 {
		t.Errorf("expected local scope to shadow outer variable")
	}
	if pkgScope.Lookup("x") != v1 {
		t.Errorf("outer scope should remain unchanged")
	}
}

func TestCheckBasicProgram(t *testing.T) {
	src := `
let x: Int = 10
var y = x + 5
func add(a: Int, b: Int) -> Int {
    return a + b
}
let total = add(a: x, b: y)
`
	file, f := parseSnippet(t, src)
	info, diags := Check([]*ast.File{file})
	if len(diags) != 0 {
		for _, d := range diags {
			t.Errorf("check diag: %s", d.Print(f))
		}
		t.Fatalf("check reported unexpected errors")
	}

	// Verify symbol definitions
	symX := info.ScopeOf(file).Lookup("x")
	if symX == nil || !types.Identical(symX.Type(), types.Typ[types.Int]) {
		t.Errorf("expected symbol x to be Int, got %v", symX)
	}

	symAdd := info.ScopeOf(file).Lookup("add")
	if symAdd == nil {
		t.Fatalf("expected symbol add to be found")
	}
	fnSym, ok := symAdd.(*FuncSymbol)
	if !ok {
		t.Fatalf("expected FuncSymbol, got %T", symAdd)
	}
	if len(fnSym.Signature().Params) != 2 {
		t.Errorf("expected 2 parameters, got %d", len(fnSym.Signature().Params))
	}
}

func TestCheckStructsAndMemberAccess(t *testing.T) {
	src := `
struct Point {
    var x: Int
    var y: Int
}
let p = Point(x: 10, y: 20)
let px = p.x
`
	file, f := parseSnippet(t, src)
	info, diags := Check([]*ast.File{file})
	if len(diags) != 0 {
		for _, d := range diags {
			t.Errorf("check diag: %s", d.Print(f))
		}
		t.Fatalf("check reported unexpected errors")
	}

	symPoint := info.ScopeOf(file).Lookup("Point")
	if symPoint == nil {
		t.Fatalf("Point type not found in scope")
	}
	st, ok := symPoint.Type().(*types.Struct)
	if !ok {
		t.Fatalf("expected Struct type, got %T", symPoint.Type())
	}
	if len(st.Fields) != 2 {
		t.Errorf("expected 2 fields, got %d", len(st.Fields))
	}
}

func TestCheckEnums(t *testing.T) {
	src := `
enum State {
    case active
    case inactive
}
`
	file, f := parseSnippet(t, src)
	info, diags := Check([]*ast.File{file})
	if len(diags) != 0 {
		for _, d := range diags {
			t.Errorf("check diag: %s", d.Print(f))
		}
		t.Fatalf("check reported unexpected errors")
	}

	symState := info.ScopeOf(file).Lookup("State")
	if symState == nil {
		t.Fatalf("State enum not found in scope")
	}
	en, ok := symState.Type().(*types.Enum)
	if !ok {
		t.Fatalf("expected Enum type, got %T", symState.Type())
	}
	if len(en.Cases) != 2 {
		t.Errorf("expected 2 enum cases, got %d", len(en.Cases))
	}
}

func TestCheckControlFlow(t *testing.T) {
	src := `
func testControlFlow(flag: Bool, count: Int) {
    if flag {
        let inside = 1
    } else {
        let other = 2
    }
    while flag {
        let w = 3
    }
    for item in [1, 2, 3] {
        let elem = item
    }
}
`
	file, f := parseSnippet(t, src)
	_, diags := Check([]*ast.File{file})
	if len(diags) != 0 {
		for _, d := range diags {
			t.Errorf("check diag: %s", d.Print(f))
		}
		t.Fatalf("check reported unexpected errors")
	}
}

func TestCheckErrorDiagnostics(t *testing.T) {
	// 1. Undeclared identifier
	src1 := `let a = unknownVariable`
	file1, _ := parseSnippet(t, src1)
	_, diags1 := Check([]*ast.File{file1})
	if len(diags1) == 0 {
		t.Errorf("expected diagnostic for undeclared identifier")
	}

	// 2. Type mismatch in variable assignment
	src2 := `let b: Int = "string"`
	file2, _ := parseSnippet(t, src2)
	_, diags2 := Check([]*ast.File{file2})
	if len(diags2) == 0 {
		t.Errorf("expected diagnostic for type mismatch")
	}

	// 3. Condition must be Bool
	src3 := `if 123 { let z = 1 }`
	file3, _ := parseSnippet(t, src3)
	_, diags3 := Check([]*ast.File{file3})
	if len(diags3) == 0 {
		t.Errorf("expected diagnostic for non-boolean condition")
	}

	// 4. Return type mismatch
	src4 := `func badReturn() -> Int { return "hello" }`
	file4, _ := parseSnippet(t, src4)
	_, diags4 := Check([]*ast.File{file4})
	if len(diags4) == 0 {
		t.Errorf("expected diagnostic for return type mismatch")
	}

	// 5. Argument label mismatch
	src5 := `
func greet(name: String) {}
greet(wrong: "Alice")
`
	file5, _ := parseSnippet(t, src5)
	_, diags5 := Check([]*ast.File{file5})
	if len(diags5) == 0 {
		t.Errorf("expected diagnostic for argument label mismatch")
	}
}

func TestCheckMultiFile(t *testing.T) {
	file1, f1 := parseSnippet(t, `
struct Vector {
    var x: Int
    var y: Int
}
`)
	file2, _ := parseSnippet(t, `
let v = Vector(x: 10, y: 20)
let sum = v.x + v.y
`)
	info, diags := Check([]*ast.File{file1, file2})
	if len(diags) != 0 {
		for _, d := range diags {
			t.Errorf("multi-file diag: %s", d.Print(f1))
		}
		t.Fatalf("unexpected diagnostics in multi-file check")
	}
	symVec := info.ScopeOf(file1).Lookup("Vector")
	if symVec == nil {
		t.Fatalf("Vector symbol not found in scope")
	}
}

func TestCheckCollectionsAndSubscripts(t *testing.T) {
	src := `
let numbers = [1, 2, 3, 4]
let first = numbers[0]

let dict = ["alpha": 1, "beta": 2]
let val = dict["alpha"]
`
	file, f := parseSnippet(t, src)
	info, diags := Check([]*ast.File{file})
	if len(diags) != 0 {
		for _, d := range diags {
			t.Errorf("check diag: %s", d.Print(f))
		}
		t.Fatalf("unexpected diagnostics")
	}
	_ = info
}

func TestCheckCastsAndConditionals(t *testing.T) {
	src := `
let x = 10
let isInt = x is Int
let y = x as Int
let z = true ? 1 : 2
`
	file, f := parseSnippet(t, src)
	_, diags := Check([]*ast.File{file})
	if len(diags) != 0 {
		for _, d := range diags {
			t.Errorf("check diag: %s", d.Print(f))
		}
		t.Fatalf("unexpected diagnostics")
	}
}

func TestCheckExtensionsAndProtocols(t *testing.T) {
	src := `
struct Point {
    var x: Int
    var y: Int
}

protocol Describable {
    func describe() -> Int
}

extension Point: Describable {
    func describe() -> Int {
        return x + y
    }
}

let p = Point(x: 1, y: 2)
let d = p.describe()
`
	file, f := parseSnippet(t, src)
	info, diags := Check([]*ast.File{file})
	if len(diags) != 0 {
		for _, d := range diags {
			t.Errorf("extension check diag: %s", d.Print(f))
		}
		t.Fatalf("unexpected diagnostics")
	}
	_ = info
}

func TestCheckProtocolConformanceErrors(t *testing.T) {
	src := `
protocol Greetable {
    func greet() -> Int
}

struct User: Greetable {
    var id: Int
}
`
	file, _ := parseSnippet(t, src)
	_, diags := Check([]*ast.File{file})
	if len(diags) == 0 {
		t.Fatalf("expected protocol conformance error, got none")
	}
	found := false
	for _, d := range diags {
		if d.Message != "" && (contains(d.Message, "does not conform to protocol") || contains(d.Message, "missing")) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected conformance error message, got: %v", diags)
	}
}

func TestCheckDefiniteInitializationAndImmutability(t *testing.T) {
	// 1. Reassigning a let constant
	srcLet := `
let x = 10
x = 20
`
	fileLet, _ := parseSnippet(t, srcLet)
	_, diagsLet := Check([]*ast.File{fileLet})
	if len(diagsLet) == 0 {
		t.Fatalf("expected error on let reassignment, got none")
	}

	// 2. Compound assignment on a let constant
	srcComp := `
let y = 10
y += 5
`
	fileComp, _ := parseSnippet(t, srcComp)
	_, diagsComp := Check([]*ast.File{fileComp})
	if len(diagsComp) == 0 {
		t.Fatalf("expected error on let compound assignment, got none")
	}

	// 3. Reading uninitialized variable
	srcUninit := `
var z: Int
let read = z + 1
`
	fileUninit, _ := parseSnippet(t, srcUninit)
	_, diagsUninit := Check([]*ast.File{fileUninit})
	if len(diagsUninit) == 0 {
		t.Fatalf("expected error on uninitialized read, got none")
	}

	// 4. Delayed initialization then reading is valid
	srcDelayed := `
var w: Int
w = 42
let readW = w + 1
let c: Int
c = 100
let readC = c + 1
`
	fileDelayed, fDelayed := parseSnippet(t, srcDelayed)
	_, diagsDelayed := Check([]*ast.File{fileDelayed})
	if len(diagsDelayed) != 0 {
		for _, d := range diagsDelayed {
			t.Errorf("unexpected diag: %s", d.Print(fDelayed))
		}
		t.Fatalf("unexpected diagnostics on valid delayed initialization")
	}

	// 5. Mutating delayed-initialized let constant
	srcReinit := `
let c: Int
c = 100
c = 200
`
	fileReinit, _ := parseSnippet(t, srcReinit)
	_, diagsReinit := Check([]*ast.File{fileReinit})
	if len(diagsReinit) == 0 {
		t.Fatalf("expected error on second let assignment, got none")
	}
}

func TestCheckSwitchExhaustiveness(t *testing.T) {
	// 1. Non-exhaustive switch reports missing cases
	srcNonExhaustive := `
enum Direction {
    case north
    case south
    case east
    case west
}

func test(d: Direction) {
    switch d {
    case .north:
        return
    case .south:
        return
    }
}
`
	fileNonEx, _ := parseSnippet(t, srcNonExhaustive)
	_, diagsNonEx := Check([]*ast.File{fileNonEx})
	if len(diagsNonEx) == 0 {
		t.Fatalf("expected exhaustiveness error, got none")
	}

	// 2. Exhaustive switch covering all cases passes
	srcExhaustive := `
enum Direction {
    case north
    case south
    case east
    case west
}

func test(d: Direction) {
    switch d {
    case .north:
        return
    case .south:
        return
    case .east:
        return
    case .west:
        return
    }
}
`
	fileEx, fEx := parseSnippet(t, srcExhaustive)
	_, diagsEx := Check([]*ast.File{fileEx})
	if len(diagsEx) != 0 {
		for _, d := range diagsEx {
			t.Errorf("unexpected diag: %s", d.Print(fEx))
		}
		t.Fatalf("exhaustive switch should not report diagnostics")
	}

	// 3. Switch with default passes
	srcDefault := `
enum Direction {
    case north
    case south
    case east
    case west
}

func test(d: Direction) {
    switch d {
    case .north:
        return
    default:
        return
    }
}
`
	fileDef, fDef := parseSnippet(t, srcDefault)
	_, diagsDef := Check([]*ast.File{fileDef})
	if len(diagsDef) != 0 {
		for _, d := range diagsDef {
			t.Errorf("unexpected diag: %s", d.Print(fDef))
		}
		t.Fatalf("switch with default should not report diagnostics")
	}
}

func TestCheckGenerics(t *testing.T) {
	src := `
let arr: Array<Int> = [1, 2, 3]
let opt: Optional<Int> = 42
let dict: Dictionary<String, Int> = ["count": 5]
`
	file, f := parseSnippet(t, src)
	_, diags := Check([]*ast.File{file})
	if len(diags) != 0 {
		for _, d := range diags {
			t.Errorf("generics check diag: %s", d.Print(f))
		}
		t.Fatalf("unexpected diagnostics on generics check")
	}
}

func TestCheckOwnershipAndMoveSemantics(t *testing.T) {
	// 1. Use after consume fails
	srcFail := `
func testUseAfterConsume() {
    var x = 10
    let y = consume x
    let z = x + 1
}
`
	fileFail, _ := parseSnippet(t, srcFail)
	_, diagsFail := Check([]*ast.File{fileFail})
	if len(diagsFail) == 0 {
		t.Fatalf("expected error on use after consume, got none")
	}
	found := false
	for _, d := range diagsFail {
		if contains(d.Message, "after consume") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected 'after consume' in error, got: %v", diagsFail)
	}

	// 2. Reassignment after consume restores variable
	srcRestore := `
func testReassignAfterConsume() {
    var x = 10
    let y = consume x
    x = 20
    let z = x + 1
}
`
	fileRestore, fRestore := parseSnippet(t, srcRestore)
	_, diagsRestore := Check([]*ast.File{fileRestore})
	if len(diagsRestore) != 0 {
		for _, d := range diagsRestore {
			t.Errorf("unexpected diag: %s", d.Print(fRestore))
		}
		t.Fatalf("reassignment after consume should restore the variable")
	}

	// 3. Double consume fails
	srcDouble := `
func testDoubleConsume() {
    var a = 5
    let b = consume a
    let c = consume a
}
`
	fileDouble, _ := parseSnippet(t, srcDouble)
	_, diagsDouble := Check([]*ast.File{fileDouble})
	if len(diagsDouble) == 0 {
		t.Fatalf("expected error on double consume, got none")
	}

	// 4. Borrow and Copy expressions
	srcBorrowCopy := `
func testBorrowAndCopy() {
    var p = 100
    let b = copy p
    let c = p
}
`
	fileBC, fBC := parseSnippet(t, srcBorrowCopy)
	_, diagsBC := Check([]*ast.File{fileBC})
	if len(diagsBC) != 0 {
		for _, d := range diagsBC {
			t.Errorf("unexpected diag: %s", d.Print(fBC))
		}
		t.Fatalf("unexpected diagnostics on copy/borrow check")
	}
}

func TestCheckActorIsolation(t *testing.T) {
	// 1. Synchronous property access without await fails
	srcPropFail := `
actor BankAccount {
    var balance: Int
    func deposit(amount: Int) -> Int {
        balance += amount
        return balance
    }
}

func testSyncAccess(acc: BankAccount) {
    let b = acc.balance
}
`
	filePropFail, _ := parseSnippet(t, srcPropFail)
	_, diagsPropFail := Check([]*ast.File{filePropFail})
	if len(diagsPropFail) == 0 {
		t.Fatalf("expected error on synchronous actor property access, got none")
	}
	found := false
	for _, d := range diagsPropFail {
		if contains(d.Message, "actor-isolated property") && contains(d.Message, "await") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected actor-isolated property error, got: %v", diagsPropFail)
	}

	// 2. Synchronous method call without await fails
	srcMethodFail := `
actor BankAccount {
    var balance: Int
    func deposit(amount: Int) -> Int {
        balance += amount
        return balance
    }
}

func testSyncCall(acc: BankAccount) {
    let res = acc.deposit(amount: 50)
}
`
	fileMethodFail, _ := parseSnippet(t, srcMethodFail)
	_, diagsMethodFail := Check([]*ast.File{fileMethodFail})
	if len(diagsMethodFail) == 0 {
		t.Fatalf("expected error on synchronous actor method call, got none")
	}
	foundMethod := false
	for _, d := range diagsMethodFail {
		if contains(d.Message, "actor-isolated method") && contains(d.Message, "await") {
			foundMethod = true
			break
		}
	}
	if !foundMethod {
		t.Fatalf("expected actor-isolated method error, got: %v", diagsMethodFail)
	}

	// 3. Asynchronous access with await succeeds
	srcAwaitSuccess := `
actor BankAccount {
    var balance: Int
    func deposit(amount: Int) -> Int {
        balance += amount
        return balance
    }
}

func testAsyncAccess(acc: BankAccount) async {
    let b = await acc.balance
    let res = await acc.deposit(amount: 50)
}
`
	fileAwait, fAwait := parseSnippet(t, srcAwaitSuccess)
	_, diagsAwait := Check([]*ast.File{fileAwait})
	if len(diagsAwait) != 0 {
		for _, d := range diagsAwait {
			t.Errorf("unexpected diag: %s", d.Print(fAwait))
		}
		t.Fatalf("await actor access should not produce diagnostics")
	}
}

func TestCheckClosureInference(t *testing.T) {
	src := `
func apply(fn: (Int) -> Int, val: Int) -> Int {
    return fn(val)
}

let r1 = apply(fn: { $0 * 2 }, val: 10)
let r2 = apply(fn: { x in x + 5 }, val: 20)
`
	file, f := parseSnippet(t, src)
	_, diags := Check([]*ast.File{file})
	if len(diags) != 0 {
		for _, d := range diags {
			t.Errorf("closure inference diag: %s", d.Print(f))
		}
		t.Fatalf("unexpected diagnostics on closure inference")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || (len(s) > 0 && len(substr) > 0 && containsSubstr(s, substr)))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// checkSnippet parses and checks src, returning the messages reported.
func checkSnippet(t *testing.T, src string) []string {
	t.Helper()
	file, _ := parseSnippet(t, src)
	_, diags := Check([]*ast.File{file})
	msgs := make([]string, len(diags))
	for i, d := range diags {
		msgs[i] = d.Message
	}
	return msgs
}

// TestCheckScopes holds the checker to Swift's rules about where a
// name lives: a nested scope may shadow, one scope may not declare
// twice, a condition's binding belongs to the body it guards, and a
// guard's binding outlives the guard but not into its else block.
//
// Every case below was run past swiftc first; the verdicts are its.
func TestCheckScopes(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string // the message expected, or "" for a clean check
	}{
		{"redeclared at file scope", "let a = 1\nlet a = 2\n",
			"invalid redeclaration of 'a'"},
		{"redeclared as a var", "func f() { let a = 1; var a = 2; _ = a }\n",
			"invalid redeclaration of 'a'"},
		{"a body may shadow a parameter", "func f(a: Int) { let a = 2; _ = a }\n", ""},
		{"a block may shadow", "func f() { let a = 1; if true { let a = 2; _ = a }; _ = a }\n", ""},
		{"if-let may shadow what it unwraps", "func f(_ o: Int?) { if let o = o { _ = o } }\n", ""},
		{"a loop variable may be rebound", "func f() { for i in [1, 2] { let i = i; _ = i } }\n", ""},
		{"an if-let binding ends with the if",
			"func f(_ o: Int?) { if let x = o { _ = x }\n_ = x }\n",
			"cannot find 'x' in scope"},
		{"a guard binding outlives the guard",
			"func f(_ o: Int?) -> Int { guard let o = o else { return 0 }\nreturn o }\n", ""},
		{"a guard's else runs before the binding",
			"func f(_ o: Int?) -> Int { guard let v = o else { return v }\nreturn v }\n",
			"cannot find 'v' in scope"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			msgs := checkSnippet(t, c.src)
			if c.want == "" {
				if len(msgs) != 0 {
					t.Errorf("expected a clean check, got %v", msgs)
				}
				return
			}
			for _, m := range msgs {
				if m == c.want {
					return
				}
			}
			t.Errorf("expected %q, got %v", c.want, msgs)
		})
	}
}

// TestCheckOneErrorPerMistake covers the diagnostics that follow from
// a type that did not resolve. Swift reports the mistake and stops;
// so does this.
func TestCheckOneErrorPerMistake(t *testing.T) {
	msgs := checkSnippet(t, "let x: Bogus = 1\n")
	if len(msgs) != 1 || msgs[0] != "cannot find type 'Bogus' in scope" {
		t.Errorf("expected one 'cannot find type' error, got %v", msgs)
	}
	msgs = checkSnippet(t, "let y = someUndefined + 1\n")
	if len(msgs) != 1 || msgs[0] != "cannot find 'someUndefined' in scope" {
		t.Errorf("expected one 'cannot find' error, got %v", msgs)
	}
}

// TestAssignmentSuppliesItsContext: the destination of an assignment
// is the context its source is read in, which is the only thing that
// gives an untyped literal or a leading-dot member a type at all.
//
// Every verdict below is swiftc's, run on the same source.
func TestAssignmentSuppliesItsContext(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string // the message expected, or "" for a clean check
	}{
		{"a literal takes the destination's width", `
func f() {
    var n: Int32 = 0
    n = 1
}
`, ""},
		{"and through a compound assignment", `
func f() {
    var n: Int32 = 0
    n += 1
}
`, ""},
		{"a leading dot names a case of the destination", `
enum Color { case red, green }
func f() {
    var c = Color.red
    c = .green
}
`, ""},
		{"a case the type does not have is still reported", `
enum Color { case red, green }
func f() {
    var c = Color.red
    c = .blue
}
`, "type 'Color' has no member 'blue'"},
		{"a leading dot with no context to resolve against", `
func f() {
    let x = .red
    _ = x
}
`, "reference to member 'red' cannot be resolved without a contextual type"},
		{"the destination's type is still enforced", `
func f() {
    var n: Int32 = 0
    n = "x"
}
`, "cannot assign value of type 'String' to type 'Int32'"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := checkSnippet(t, tc.src)
			if tc.want == "" {
				if len(got) != 0 {
					t.Errorf("reported %v, want a clean check", got)
				}
				return
			}
			for _, m := range got {
				if strings.Contains(m, tc.want) {
					return
				}
			}
			t.Errorf("reported %v, want one containing %q", got, tc.want)
		})
	}
}

// TestSwitchPatternsAreChecked: `case 0:` is a value compared with the
// subject, so it is checked against the subject's type. Before it was,
// the pattern had no recorded type at all and the whole switch was
// dropped on the floor by the code generator.
func TestSwitchPatternsAreChecked(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"a literal pattern matching its subject", `
func f(_ n: Int32) {
    switch n {
    case 0: break
    default: break
    }
}
`, ""},
		{"a pattern of the wrong type", `
func f(_ n: Int32) {
    switch n {
    case "x": break
    default: break
    }
}
`, "cannot match values of type 'Int32'"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := checkSnippet(t, tc.src)
			if tc.want == "" {
				if len(got) != 0 {
					t.Errorf("reported %v, want a clean check", got)
				}
				return
			}
			for _, m := range got {
				if strings.Contains(m, tc.want) {
					return
				}
			}
			t.Errorf("reported %v, want one containing %q", got, tc.want)
		})
	}
}

// TestRangesAreTyped: a range is its two bounds, and they have to be
// one type. The element is what a for-in binds its variable to, which
// is the only thing that gave the variable a type at all — before
// this, a range was modelled as a name whose underlying type was its
// lower bound, so `0..<n` looked like an Int to everything that asked.
func TestRangesAreTyped(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"the literal takes the other bound's type", `
func f(_ n: Int32) {
    for i in 0..<n { _ = i }
}
`, ""},
		{"bounds that disagree", `
func f(_ a: Int32, _ b: Int) {
    for i in a..<b { _ = i }
}
`, "cannot form a range from 'Int32' to 'Int'"},
		{"the loop variable is the bound's type, not the sequence's", `
func f(_ n: Int32) {
    var t: Int32 = 0
    for i in 0..<n { t = t + i }
}
`, ""},
		{"and a mismatch against it is still reported", `
func f(_ n: Int32) {
    var t: Int = 0
    for i in 0..<n { t = t + i }
}
`, "cannot be applied to operands of type 'Int' and 'Int32'"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := checkSnippet(t, tc.src)
			if tc.want == "" {
				if len(got) != 0 {
					t.Errorf("reported %v, want a clean check", got)
				}
				return
			}
			for _, m := range got {
				if strings.Contains(m, tc.want) {
					return
				}
			}
			t.Errorf("reported %v, want one containing %q", got, tc.want)
		})
	}
}

// TestArgumentLabelsAreNotPartOfTheType: SE-0111 took argument labels
// out of Swift's type system, so a declared function is assignable to
// a variable of its type however its parameters were labelled. They
// are still part of the declaration's full name, which is what lets
// two functions differ in labels alone -- two questions that cannot
// share an answer, and did.
//
// Both verdicts are swiftc's, run on the same source.
func TestArgumentLabelsAreNotPartOfTheType(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"a labelled function is a value of its unlabelled type", `
func triple(_ n: Int32) -> Int32 { return n * 3 }
func named(x: Int32) -> Int32 { return x }
func f() {
    let a: (Int32) -> Int32 = triple
    let b: (Int32) -> Int32 = named
    _ = a
    _ = b
}
`, ""},
		{"two declarations may differ in labels alone", `
func label(a: Int) -> Int { return a }
func label(b: Int) -> Int { return b }
func f() {
    _ = label(a: 1)
    _ = label(b: 2)
}
`, ""},
		{"and the same full name twice is still a redeclaration", `
func label(a: Int) -> Int { return a }
func label(a: Int) -> Int { return a }
`, "invalid redeclaration of 'label'"},
		{"a function of the wrong type is still rejected", `
func triple(_ n: Int32) -> Int32 { return n * 3 }
func f() {
    let a: (Int) -> Int = triple
    _ = a
}
`, "cannot convert"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := checkSnippet(t, tc.src)
			if tc.want == "" {
				if len(got) != 0 {
					t.Errorf("reported %v, want a clean check", got)
				}
				return
			}
			for _, m := range got {
				if strings.Contains(m, tc.want) {
					return
				}
			}
			t.Errorf("reported %v, want one containing %q", got, tc.want)
		})
	}
}

// checkFiles checks several files as one module, the way a build of
// more than one source does.
func checkFiles(t *testing.T, srcs ...string) []string {
	t.Helper()
	files := make([]*ast.File, len(srcs))
	for i, src := range srcs {
		f := token.NewFile("file"+itoaTest(i)+".swift", []byte(src))
		file, diags := parser.ParseFile(f, 0)
		if len(diags) != 0 {
			for _, d := range diags {
				t.Errorf("parser diag: %s", d.Print(f))
			}
			t.Fatalf("parse failed: %s", src)
		}
		files[i] = file
	}
	_, diags := Check(files)
	msgs := make([]string, len(diags))
	for i, d := range diags {
		msgs[i] = d.Message
	}
	return msgs
}

func itoaTest(n int) string { return string(rune('0' + n)) }

// TestAccessControlAcrossFiles: a declaration's access level says
// which files may name it, and nothing was checking.
//
// This is the rule imports are built on. If `private` is not enforced
// between two files of one module, `public` will not be enforced
// between two modules -- the module interface would be describing a
// boundary that nothing holds anyone to.
//
// Every verdict below is swiftc's, run on the same two files.
func TestAccessControlAcrossFiles(t *testing.T) {
	const lib = `
private func priv() -> Int32 { return 1 }
fileprivate func fpriv() -> Int32 { return 2 }
internal func intern() -> Int32 { return 4 }
public func pub() -> Int32 { return 8 }
func implicit() -> Int32 { return 16 }

private struct PrivType { var n: Int32 }
public struct PubType { var n: Int32 }
`
	cases := []struct {
		name string
		use  string
		want string
	}{
		{"private is not visible in another file",
			"func f() -> Int32 { return priv() }",
			"'priv' is inaccessible due to 'private' protection level"},
		{"nor is fileprivate",
			"func f() -> Int32 { return fpriv() }",
			"'fpriv' is inaccessible due to 'fileprivate' protection level"},
		{"internal is the module, and one file is in it",
			"func f() -> Int32 { return intern() }", ""},
		{"public certainly is",
			"func f() -> Int32 { return pub() }", ""},
		{"and a declaration that says nothing is internal",
			"func f() -> Int32 { return implicit() }", ""},
		{"a private type is no more visible than a private function",
			"func f() -> Int32 { return PrivType(n: 1).n }",
			"'PrivType' is inaccessible due to 'private' protection level"},
		{"a public one is",
			"func f() -> Int32 { return PubType(n: 1).n }", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := checkFiles(t, lib, tc.use)
			if tc.want == "" {
				if len(got) != 0 {
					t.Errorf("reported %v, want a clean check", got)
				}
				return
			}
			for _, m := range got {
				if strings.Contains(m, tc.want) {
					return
				}
			}
			t.Errorf("reported %v, want one containing %q", got, tc.want)
		})
	}
}

// TestAccessControlWithinAFile: the same declarations are visible to
// the file that wrote them, which is the half that must not break.
func TestAccessControlWithinAFile(t *testing.T) {
	got := checkSnippet(t, `
private func priv() -> Int32 { return 1 }
fileprivate func fpriv() -> Int32 { return 2 }
private struct PrivType { var n: Int32 }

func useThem() -> Int32 {
    return priv() + fpriv() + PrivType(n: 3).n
}
`)
	if len(got) != 0 {
		t.Errorf("reported %v, want a clean check", got)
	}
}
