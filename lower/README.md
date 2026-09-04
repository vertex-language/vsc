# lower

VIL to VIR: the seam between the two halves of the compiler.

Above it everything is Swift — formal types, ownership, conventions, the
vocabulary a diagnostic can be written in. Below it everything is machine:
registers of seven widths, memory reached through pointers, and no notion at
all of what a value means. VIR is shared with `vcc` and `v++`, so nothing
Swift-shaped may cross.

## What arrives here

A module in `vil.StageLowered` — ownership already erased by `vil/pass`. A
module still in the ownership form is refused rather than lowered with its
retains dropped on the floor.

## What a type becomes

| Swift | VIR |
|---|---|
| `Bool` | `i1` |
| `Int`, `Int64`, `UInt`, `UInt64` | `i64` |
| `Int32`, `UInt32` | `i32` |
| `Int8`, `Int16` (and unsigned) | `i32`, always extended from its own width |
| `Float`, `Double` | `f32`, `f64` |
| a class reference, a thin function, a metatype | `ptr` |
| an address of anything | `ptr` |

`Int8` and `Int16` have no registers of their own: §2 of the VIR spec makes
those storage-only widths. They are held in `i32` under an invariant — the
register always holds a value already extended from the narrow width — which
arithmetic restores afterwards, and whose restoration is also how the overflow
is detected.

Everything else is held in memory, and this package says so rather than
guessing a layout.

## Where Swift and VIR disagree, and who wins

- **No greater-than.** §L gives `lt` and `le` and no `gt`, because every target
  has one comparison and a choice of which way to read it. `cmp_sgt_Int64(a, b)`
  becomes `i64.slt b, a`.
- **No unsigned subtract-with-overflow.** An unsigned subtraction overflows
  exactly when it borrows, which is `a < b`.
- **Overflow is a separate instruction.** Swift's `sadd_with_overflow` returns a
  pair; VIR has `i64.add` and `i64.saddo`, and both are emitted.
- **`cond_fail` is a trap.** The block splits, the failing edge goes to a block
  that traps, and there is nothing to unwind to — which is what Swift's
  precondition failure is.

## What it refuses

A great deal, and each refusal names the instruction and the function it could
not translate. The alternative to refusing is emitting something that runs and
is wrong.

The current list, in the order it is worth fixing:

- a struct or tuple of more than one field held in a register — Swift passes
  small aggregates in several registers, and that is the next real piece of work
- boxes (`alloc_box`, `project_box`), which need the allocator the runtime
  provides
- enums with payloads, `switch_enum`
- existentials, witness tables, class methods, `partial_apply`
- throwing and async functions
- string literals

## The object header

A class instance begins with two words — what the object is, and how many
references are held to it — and its stored properties start after them. This is
the one place the compiler has to agree with the runtime about a number; the
runtime itself is C, compiled and linked by `vcc`.

## Held to it

`lower_test.go` runs the `vil/gen` corpus through the whole pipeline and
requires that `ir/verify` accepts what comes out, and that the two programs it
cannot lower are refused with a reason rather than lowered wrongly. `TestFib` is
the golden one: a whole Swift function, in VIR, with nothing of Swift left in it.
