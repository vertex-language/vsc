// Float arithmetic and comparisons, on values every device rounds alike.
import "gpu"

func lerp(_ a: float32, _ b: float32, _ y: gpu.MutableSpan<float32>) kernel {
    let i = gpu.Index.x
    let t = float32(i) / 4
    let v = a + (b - a) * t
    y[i] = v < 5 ? -v : v
}

let y = try await gpu.Default().Upload([float32](repeating: 0, count: 5))
try await lerp.Launch(2, 10, y, over: 5)
print(try await y.Download())
// want: [-2.0, -4.0, 6.0, 8.0, 10.0]
