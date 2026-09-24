// Inside another kernel, an element kernel is an ordinary device function.
import "gpu"

func smooth(_ a: float32, _ b: float32) kernel -> float32 {
    return (a + b) / 2
}

func pairs(_ x: gpu.Span<float32>, _ y: gpu.MutableSpan<float32>) kernel {
    let i = gpu.Index.x
    y[i] = smooth(x[i], x[i + 1])
}

let d = gpu.Default()
let x = try await d.Upload([float32(0), 2, 6, 12])
let y = try await d.Upload([float32](repeating: 0, count: 3))
try await pairs.Launch(x, y, over: 3)
print(try await y.Download())
// want: [1.0, 4.0, 9.0]
