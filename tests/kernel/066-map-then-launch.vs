// A Map's result is a buffer like any other: launched on, mapped again.
import "gpu"

func plusOne(_ x: int32) kernel -> int32 {
    return x + 1
}

func total(_ x: gpu.Span<int32>, _ out: gpu.MutableSpan<int32>) kernel {
    _ = gpu.Atomic.Add(out.Address(0), x[gpu.Index.x])
}

let d = gpu.Default()
let xs = try await d.Upload([int32](repeating: 0, count: 100))
let ys = try await plusOne.Map(try await plusOne.Map(xs))
let sum = try await d.Upload([int32(0)])
try await total.Launch(ys, sum, over: ys.count)
print(try await sum.Download())
// want: [200]
