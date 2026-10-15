package vsc_test

import (
	"testing"

	"github.com/vertex-language/ir"
	"github.com/vertex-language/vsc"
)

// Under SysV a result of more than two words comes back through memory,
// where AAPCS64 returns four in registers: a call's enum with a String
// payload, made into a tuple an async function returns, was an operand
// held in memory that the tuple could not take ("an operand held in
// memory: in recv: tuple"). On a Mac it never was.
func TestAsyncTupleOfAWideResultForSysV(t *testing.T) {
	src := `
enum Addr { case v4(ip: String, port: UInt16); case v6(ip: String, port: UInt16) }
func make(_ n: Int) -> Addr { return n > 1 ? .v6(ip: "::1", port: UInt16(n)) : .v4(ip: "1.2.3.4", port: 7) }
func tick(_ n: Int) async -> Int { return n }
func recv(_ k: Int) async throws -> (Int, Addr) {
    let n = await tick(k)
    return (n, make(n))
}
print(try await recv(3))
`
	for _, target := range []ir.Target{ir.X86_64Linux, ir.AArch64MacOS} {
		_, diags := vsc.Compile([]vsc.Source{{Name: "main.swift", Text: []byte(src)}},
			vsc.Options{Module: "main", Target: target})
		if vsc.Errors(diags) {
			for _, d := range diags {
				t.Errorf("%s: %s", target, d)
			}
		}
	}
}
