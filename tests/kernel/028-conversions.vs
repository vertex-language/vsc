// Conversions between integer and float types inside a kernel.
import "gpu"

func conv(_ x: gpu.Span<float32>, _ y: gpu.MutableSpan<int32>) kernel {
    let i = gpu.Index.x
    let f = x[i] * 3
    let n = int32(f)
    y[i] = n + int32(int8(truncatingIfNeeded: i + 250))
}

let d = gpu.Default()
let x = try await d.Upload([float32(1.5), 2.25, -0.75])
let y = try await d.Upload([int32](repeating: 0, count: 3))
try await conv.Launch(x, y, over: 3)
print(try await y.Download())
// want: [-2, 1, -6]
