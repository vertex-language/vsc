# tests/kernel

The kernel ladder: one small thing a kernel does per file, numbered
`001`–`072` in the order the rungs climb, from an empty kernel to programs
(see `proposed_vertex_kernel.md` on the Desktop for the design).

There is no swiftc to compare with here, so each file says what it prints:

```vertex
// want: [5.0, 5.0]     a line of standard output, in order
// error: a kernel      the build fails, and says this
```

Every file that builds is run twice: on the CPU device and on Metal
(`VERTEX_GPU=cpu`, then `VERTEX_GPU=metal`), and both runs must print what
it wants. The CPU device is the oracle the GPU is held to. A build that says
anything on standard error fails too, because a kernel not built for a device
says so there and runs where it can instead.

| | Files | The oracle | Compared |
| --- | --- | --- | --- |
| `tests/kernel/` | `001`–`072` `.vs` | the file's `// want:` lines | stdout, on the CPU device and on Metal |

## `001`–`072`

| | |
| --- | --- |
| 001–008 | the smallest kernels: an empty one, one element written, `gpu.Index.x`, the index as a value, spans of `int32`, `uint32`, `int` and `uint8` |
| 009–016 | parameters and the guard: a `float32`, an `int32`, a labelled `int` and a `bool` passed by value, two spans, saxpy, a grid larger than the data, and `guard` |
| 017–029 | the language inside a kernel: locals, `for` and `while`, helper functions and chains of them, `?:` and `if`, `switch`, a struct of the program's own, tuples, integer division, float arithmetic, conversions, bits |
| 030–038 | grids and workgroups: 2D and 3D grids, `workgroup:`, `LocalIndex`, `GroupIndex`, `GroupSize`, `GroupCount`, a buffer's slice, launches in sequence, two outputs, a span's `count`, `Unchecked` |
| 039–049 | working together: `gpu.Shared` and `gpu.Barrier` (a reversal, a tree sum), atomics (add, min, max, float add, compare-exchange, the bitwise ones), and waves (`Size`, `Lane`, `Sum`, `Any`, `All`, `Min`, `Max`), each written to print the same whatever a wave's width |
| 050–052 | the host side: buffers created, filled, copied, sliced; the devices; a launch on the CPU device's buffers |
| 053–058 | what a kernel may not do, refused where it is compiled: print, `async`, a kernel called as a function, a buffer of the wrong element, a `String` parameter, an `Array` |
| 059–068 | element kernels: `Map` over one buffer and two, a broadcast argument, `into:`, element types in and out, a narrow result, an element kernel called from a grid kernel, a Map's result used again, and the two refusals |
| 069–070 | programs: a histogram through shared storage and atomics, and a matrix multiply over a 2D grid |
| 071 | barriers at scale: 64 full groups of 1024 passing 64 barriers each, which the CPU device runs as fibers |
| 072 | `int64` and `uint64` parameters |

## Rules

- **One thing per file.** A failure should name what broke.
- **Nothing that depends on the device.** A wave is 32 lanes on Apple's GPUs
  and 1 on the CPU device, so a rung that uses waves prints what is the same
  for any width. Float results are values every device rounds alike.
- **A refusal is a rung too.** `// error:` rungs hold the checks §5 of the
  design asks for to what they say.
- **Every fix lands with the smallest numbered file that shows it.**

## Running

```console
$ go test -run TestKernels .              # the ladder, from vsc's root
$ go test -run 'TestKernels/039' .        # one rung
$ vsc run tests/kernel/014-saxpy.vs       # one rung, by hand, on the default device
$ VERTEX_GPU=cpu vsc run tests/kernel/014-saxpy.vs
```

`VSC_DUMP_DEVICE=dir` writes each kernel's device VIR and Metal library to
`dir` as the compiler builds them, and `VERTEX_GPU_DEBUG=1` has the runtime
say what it does with Metal.
