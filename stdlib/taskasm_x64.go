package stdlib

import "strconv"

// The runtime's assembly for x86-64: the same jobs as the AArch64 assembly
// beside TaskAsm, under the platform's C convention with Swift's registers
// laid over it -- self in R13, the async context in R14 -- which are the
// same two on Windows and on SysV. See ir/lower/amd64's swift.go, which
// places them, and swiftc's code, which is where they were measured.
//
// Windows first. Three things about the Microsoft convention reach every
// routine there:
//
//   - The C++ that calls these passes four arguments in RCX, RDX, R8 and R9
//     and the fifth on the stack, at 40(%rsp) on entry: past the return
//     address and the 32 bytes of home space the caller reserved for the
//     four in registers.
//
//   - Every call a routine makes has to leave that home space in turn, and
//     RSP 16-byte aligned at the call. RSP is 8 past a multiple of 16 on
//     entry, since the call pushed the return address.
//
//   - R13 and R14 are callee-saved, so a routine that loads one saves it
//     first and puts it back after. The code it calls owes nothing back: a
//     continuation that tail calls hands its registers forward.

// taskAsmX64Windows is TaskAsm for x86_64-windows. No symbol prefix: a
// COFF symbol on x64 is the C name as written.
func taskAsmX64Windows() string {
	out := "\t.text\n"
	for _, r := range []struct{ name, body string }{
		{"vertex_task_enter", taskEnterX64Windows},
		{"vertex_witness_call", witnessCallX64Windows},
		{"vertex_witness_call1", witnessCall1X64Windows},
		{"vertex_witness_call_static2", witnessCallStatic2X64Windows},
	} {
		out += "\t.globl " + r.name + "\n\t.p2align 4\n" + r.name + ":" + r.body
	}
	for _, p := range asyncEntries {
		out += asyncThunkX64(msArgRegs[:], p.name, p.args, false)
	}
	return out + conformanceBracketsX64Windows
}

// conformanceBracketsX64Windows is the two ends of the grouped section
// every conformance record is in: vertex_proto_start in .vproto$A and
// vertex_proto_stop in .vproto$C, around the records every module puts in
// .vproto$B. The linker sorts a group's contributions by the text after
// the $, so everything between the two is a record, and a record is where
// the runtime looks. Each end is a zero record rather than nothing, so the
// group is never empty; a zero record is skipped. See lower's
// COFFConformanceSection and runtime/platform/windows.h.
const conformanceBracketsX64Windows = `
	.section ".vproto$A","dr"
	.p2align 2
	.globl vertex_proto_start
vertex_proto_start:
	.long 0
	.section ".vproto$C","dr"
	.p2align 2
	.globl vertex_proto_stop
vertex_proto_stop:
	.long 0
	.text
`

// taskEnterX64Windows is vertex_task_enter(code, async, self, a0, a1): the
// context into R14 and the captures into R13, a0 and a1 as the first two
// arguments, and a call to code. a1 is the fifth argument, on the stack.
//
//	entry     RSP = 8 mod 16, a1 at 40(%rsp)
//	pushes    R13 and R14 saved, RSP = 8 mod 16
//	sub 40    home space for the call and 8 to align it; a1 now at
//	          40 + 16 + 40 = 96(%rsp)
const taskEnterX64Windows = `
	pushq %r13
	pushq %r14
	subq $40, %rsp
	movq %rcx, %rax
	movq %rdx, %r14
	movq %r8, %r13
	movq %r9, %rcx
	movq 96(%rsp), %rdx
	callq *%rax
	addq $40, %rsp
	popq %r14
	popq %r13
	retq
`

// witnessCallX64Windows is vertex_witness_call(fn, self, metadata, table),
// which answers a String. Two words are wider than the Microsoft
// convention returns in registers, so the C++ caller passes the address to
// write it to first, in RCX, and everything else moves up one:
//
//	RCX result, RDX fn, R8 self, R9 metadata, 40(%rsp) table
//
// A witness answering a String takes that address the same way, as its
// first argument, then its metadata and table, with the conformer in R13.
// So RCX is already where the witness wants it. It is kept too, because
// the convention returns a result's address in RAX and the caller may
// read it there.
//
//	entry       RSP = 8 mod 16, table at 40(%rsp)
//	pushes      R13 saved and the result's address, RSP = 8 mod 16
//	sub 40      home space and alignment; table now at 40 + 16 + 40
const witnessCallX64Windows = `
	pushq %r13
	pushq %rcx
	subq $40, %rsp
	movq %r8, %r13
	movq %rdx, %rax
	movq %r9, %rdx
	movq 96(%rsp), %r8
	callq *%rax
	addq $40, %rsp
	popq %rax
	popq %r13
	retq
`

// witnessCall1X64Windows is vertex_witness_call1(fn, self, arg, metadata,
// table): the one declared argument first, the conformer in R13, and the
// metadata and table after the argument.
//
//	entry     RSP = 8 mod 16, table at 40(%rsp)
//	push      R13 saved, RSP = 0 mod 16
//	sub 32    home space; table now at 40 + 8 + 32 = 80(%rsp)
const witnessCall1X64Windows = `
	pushq %r13
	subq $32, %rsp
	movq %rcx, %rax
	movq %rdx, %r13
	movq %r8, %rcx
	movq %r9, %rdx
	movq 80(%rsp), %r8
	callq *%rax
	addq $32, %rsp
	popq %r13
	retq
`

// witnessCallStatic2X64Windows is vertex_witness_call_static2(fn, a, b,
// metadata, table): the operands' addresses first, the type in R13 -- a
// static witness's self is its metatype -- and the metadata and table after
// the operands. Its Bool comes back in AL, and stays there.
const witnessCallStatic2X64Windows = `
	pushq %r13
	subq $32, %rsp
	movq %rcx, %rax
	movq %r9, %r13
	movq %rdx, %rcx
	movq %r8, %rdx
	movq %r9, %r8
	movq 80(%rsp), %r9
	callq *%rax
	addq $32, %rsp
	popq %r13
	retq
`

// msArgRegs is the Microsoft convention's four argument registers.
var msArgRegs = [...]string{"%rcx", "%rdx", "%r8", "%r9"}

// asyncThunkX64 is one of asyncEntries' thunks: entered as an async
// function is, with the context in R14 and the arguments from the first
// argument register, and jumping to the C behind it with the context first
// and the arguments up one. A jump and not a call, so that when the C
// returns it returns to the executor that entered the task -- whose call
// left, on Windows, the home space above the return address the C is owed.
func asyncThunkX64(regs []string, name string, args int, elf bool) string {
	if args+1 > len(regs) {
		panic("stdlib: " + name + " takes " + strconv.Itoa(args) + " arguments; its thunk has registers for " + strconv.Itoa(len(regs)-1))
	}
	out := "\t.globl " + name + "\n"
	if elf {
		out += "\t.type " + name + ", @function\n"
	}
	out += "\t.p2align 4\n" + name + ":\n"
	// Downwards, so that moving one register into the next cannot
	// overwrite an argument that has not moved yet.
	for i := args; i >= 1; i-- {
		out += "\tmovq " + regs[i-1] + ", " + regs[i] + "\n"
	}
	out += "\tmovq %r14, " + regs[0] + "\n"
	out += "\tjmp " + name + "_at\n"
	return out
}

// SysV, as Linux has it. Two things differ from Windows and reach every
// routine: six argument registers -- RDI, RSI, RDX, RCX, R8, R9 -- so none of
// these takes an argument from the stack; and no home space, so a call
// needs only RSP aligned to sixteen, which one push on entry makes it. A
// result of two words comes back in RAX and RDX, where the witness a String
// getter is left it, so vertex_witness_call needs no storage of its own.

// taskAsmX64SysV is TaskAsm for x86_64-linux.
func taskAsmX64SysV() string {
	out := "\t.text\n"
	for _, r := range []struct{ name, body string }{
		{"vertex_task_enter", taskEnterX64SysV},
		{"vertex_witness_call", witnessCallX64SysV},
		{"vertex_witness_call1", witnessCall1X64SysV},
		{"vertex_witness_call_static2", witnessCallStatic2X64SysV},
	} {
		out += "\t.globl " + r.name + "\n\t.type " + r.name + ", @function\n\t.p2align 4\n" + r.name + ":" + r.body
	}
	for _, p := range asyncEntries {
		out += asyncThunkX64(sysvArgRegs[:], p.name, p.args, true)
	}
	return out + elfConformanceZero
}

// taskEnterX64SysV is vertex_task_enter(code, async, self, a0, a1): R13 and
// R14 saved, and a pad to align the call; the context into R14, the
// captures into R13, a0 and a1 as the first two arguments.
const taskEnterX64SysV = `
	pushq %r13
	pushq %r14
	subq $8, %rsp
	movq %rdi, %rax
	movq %rsi, %r14
	movq %rdx, %r13
	movq %rcx, %rdi
	movq %r8, %rsi
	callq *%rax
	addq $8, %rsp
	popq %r14
	popq %r13
	retq
`

// witnessCallX64SysV is vertex_witness_call(fn, self, metadata, table): the
// conformer in R13, the metadata and table as the witness's first two
// arguments, and its String left in RAX and RDX.
const witnessCallX64SysV = `
	pushq %r13
	movq %rdi, %rax
	movq %rsi, %r13
	movq %rdx, %rdi
	movq %rcx, %rsi
	callq *%rax
	popq %r13
	retq
`

// witnessCall1X64SysV is vertex_witness_call1(fn, self, arg, metadata,
// table): the one declared argument first, the conformer in R13, and the
// metadata and table after the argument.
const witnessCall1X64SysV = `
	pushq %r13
	movq %rdi, %rax
	movq %rsi, %r13
	movq %rdx, %rdi
	movq %rcx, %rsi
	movq %r8, %rdx
	callq *%rax
	popq %r13
	retq
`

// witnessCallStatic2X64SysV is vertex_witness_call_static2(fn, a, b,
// metadata, table): the operands' addresses first, the type in R13 -- a
// static witness's self is its metatype -- and the metadata and table
// after the operands. Each register is read before it is written.
const witnessCallStatic2X64SysV = `
	pushq %r13
	movq %rdi, %rax
	movq %rcx, %r13
	movq %rsi, %rdi
	movq %rdx, %rsi
	movq %rcx, %rdx
	movq %r8, %rcx
	callq *%rax
	popq %r13
	retq
`

// sysvArgRegs is the SysV convention's six integer argument registers.
var sysvArgRegs = [...]string{"%rdi", "%rsi", "%rdx", "%rcx", "%r8", "%r9"}
