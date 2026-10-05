// An atomic float add, of values whose sum is exact in any order.
import "gpu"

func sum(_ x: gpu.Span<float32>, _ out: gpu.MutableSpan<float32>) kernel {
    _ = gpu.Atomic.Add(out.Address(0), x[gpu.Index.x])
}

let d = gpu.Default()
let x = try await d.Upload([float32](repeating: 0.5, count: 256))
let out = try await d.Upload([float32(0)])
try await sum.Launch(x, out, over: 256)
print(try await out.Download())
// want: [128.0]
