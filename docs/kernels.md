# Kernels

`kernel` goes where `async` goes: `func f(...) kernel`. A kernel that returns
nothing is a **grid kernel**, called once per work-item. A kernel that returns
a value is an **element kernel**, called once per element and applied with
`Map`. `import "gpu"` gives both the device API and the host API. `gpu` is
built into the compiler, so there's nothing to install.

```swift
import "gpu"

// A grid kernel: one call per work-item.
func saxpy(a: float32, x: gpu.Span<float32>, y: gpu.MutableSpan<float32>) kernel {
    let i = gpu.Index.x
    if i < y.count {
        y[i] = a * x[i] + y[i]
    }
}

// An element kernel: one output element per call.
func gelu(x: float32) kernel -> float32 {
    let t = 0.7978845608 * (x + 0.044715 * x * x * x)
    return 0.5 * x * (1 + tanh(t))
}

// An ordinary function: compiled for the device because a kernel calls it.
func tanh(t: float32) -> float32 {
    let t2 = t * t
    return t * (27 + t2) / (27 + 9 * t2)
}

// A group reduction: shared storage and barriers, then one wave in registers.
func sum(x: gpu.Span<float32>, out: gpu.MutableSpan<float32>) kernel {
    let partial = gpu.Shared<float32>(count: 256)
    let me = gpu.LocalIndex.x
    let i = gpu.Index.x
    partial[me] = i < x.count ? x[i] : 0
    gpu.Barrier()
    var stride = gpu.GroupSize.x / 2
    while stride >= gpu.Wave.Size {
        if me < stride {
            partial[me] += partial[me + stride]
        }
        gpu.Barrier()
        stride /= 2
    }
    if me < gpu.Wave.Size {
        let s = gpu.Wave.Sum(partial[me])
        if gpu.Wave.Lane == 0 {
            _ = gpu.Atomic.Add(out.Address(0), s)
        }
    }
}

// A generic kernel: built for each element type it is launched with.
func scale<T: Numeric>(y: gpu.MutableSpan<T>, k: T) kernel {
    let i = gpu.Index.x
    if i < y.count {
        y[i] = y[i] * k
    }
}

let device = gpu.Default()                                   // Metal here, or the CPU
let n = 1 << 16
let x = try await device.Upload([float32](repeating: 1, count: n))
let y = try await device.Upload([float32](repeating: 2, count: n))

try await saxpy.Launch(3.0, x, y, over: n)                   // the grid is the work
let acts = try await gelu.Map(y)                             // a buffer in, a new buffer out

let total = try await device.Upload([float32(0)])
try await sum.Launch(y, total, over: n, workgroup: 256)
print(try await total.Download()[0])                         // 327680.0

let ints = try await device.Upload([int32(1), 2, 3])
try await scale.Launch(ints, 10, over: 3)                    // [10, 20, 30]

// The CPU device is always there: the oracle a GPU result is checked against.
let input: [float32] = [-2, -0.5, 0, 0.5, 2]
let onGPU = try await gelu.Map(try await device.Upload(input)).Download()
let onCPU = try await gelu.Map(try await gpu.CPU().Upload(input)).Download()
print(onGPU == onCPU)                                        // true
```

## What the compiler does for you

- **Typed launches.** Every kernel gets a `Launch` method with the kernel's own
  parameters, and every element kernel gets `Map`. A `gpu.Span<T>` parameter
  takes a `gpu.Buffer<T>`. A wrong argument is a compile error, not a crash on
  the device.
- **No device annotations.** Anything a kernel calls is compiled for the device
  and checked there: plain functions, generic functions, other packages'
  `@inlinable` functions, and element kernels called directly.
- **The rules are checked, with the chain.** A kernel can't reach the heap,
  `print`, `throws`, `async`, or a global. The error says how it got there:
  `kernel 'noisy' cannot run on a device: noisy calls log`. Calling a kernel
  like a function is refused too: `'fill' is a kernel: launch it with
  fill.Launch`.
- **Memory spaces are inferred.** Spans are device memory, scalars are
  constants, `gpu.Shared` is workgroup memory, and locals are private. You
  never write an address space.
- **Bounds are checked.** `span[i]` traps outside the span, and the
  `if i < y.count` guard lets the compiler drop the check beneath it.
  `span.Unchecked[i]` removes it visibly at one access.

## The device API

| API | Is |
| :--- | :--- |
| `gpu.Index`, `LocalIndex`, `GroupIndex` | where this work-item is: in the grid, in its group, and which group (`.x`, `.y`, `.z`) |
| `gpu.GroupSize`, `GroupCount`, `GridSize` | work-items per group, groups in the grid, work-items in the grid |
| `gpu.Span<T>`, `gpu.MutableSpan<T>` | views of device memory: `count`, a subscript, `Slice`, `Unchecked`, `Address` |
| `gpu.Shared<T>(count:)` | one allocation per workgroup, written where it's used |
| `gpu.Barrier()` | every work-item in the group arrives, and memory is visible after it |
| `gpu.Wave` | `Lane`, `Size`, `Shuffle…`, `Any`, `All`, `Ballot`, `First`, `Sum`, `Min`, `Max` |
| `gpu.Atomic` | `Add`, `Sub`, `Min`, `Max`, `And`, `Or`, `Xor`, `Exchange`, `CompareExchange`, on integers and floats |

### Host and buffer operations

- **Devices**: `gpu.Default()`, `gpu.CPU()`, `gpu.Devices()`
- **Allocations**: `device.Upload`, `device.CreateBuffer`, `device.Wrap` (zero-copy wrapping of existing host memory)
- **Buffer operations**: `Download`, `Copy`, `Fill`, `Slice`, and `View(as:)`

### Rendering on Metal

On CPU devices, these calls are no-ops and callers draw on CPU. On Metal:

- **Textures**: `gpu.Texture.Create` (2D or cube; RGBA8, RGBA32F, or depth-stencil; mip levels) with `Write` and `Read`.
- **Pipelines**: `gpu.Library(device:source:)` (runtime-compiled Metal source) and its `Function`; `gpu.RenderPipeline` from a `PipelineDescriptor` (formats, blending, write mask, vertex layout); `DepthStencilState.Create`, `SamplerState.Create`.
- **Render passes**: `RenderPass.Begin` with `SetPipeline`, `SetViewport`, `SetScissor`, `SetRaster`, `SetBytes`, `SetTexture`, `SetSampler`, `Draw`, `DrawIndexed`, and `End`.
- **Execution**: Passes record into a single command buffer executed by `device.Commit`. Texture `Read`/`Write` waits for committed work, while `Upload` stages and copies asynchronously. `gpu/raster` uses these primitives to draw.
- **Window presentation**: `gpu.Layer.Attach(device:view:width:height:scale:)` lays a `CAMetalLayer` over a window view (`window.Surface.RawParts`). `Layer.Present(scene, overlay:, backdrop:)` composites directly on the GPU without CPU readback:
  - The scene texture scaled to the window.
  - An optional frosted `Backdrop` (a rounded rectangle blurred with the background dimmed).
  - An optional premultiplied overlay (HUD).
  - `Layer.Capture` reads back the same composite for screenshots. `entity/run`'s `Window(…, present:, …)` runs a program this way.

## Devices

| Device | State |
| :--- | :--- |
| **Metal** (Apple GPUs) | Runs. Kernels are lowered to AIR and embedded as Metal libraries. |
| **CPU** | Runs everywhere. Groups run on a worker per core, and a group that uses barriers runs as fibers on arm64, so a barrier is a stack switch. A `float64` kernel, which Apple GPUs can't run, falls back to it. |
| **NVIDIA**, **AMD** | Kernel lowering to PTX and AMDGPU exists in `ir`. The driver runtime doesn't load them yet. |

`vsc/tests/kernel` is the kernel test suite: 80 programs, each run on the CPU
device and on Metal and compared.
