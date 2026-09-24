// Two buffers: read one, write the other.
import "gpu"

func copy(_ x: gpu.Span<float32>, _ y: gpu.MutableSpan<float32>) kernel {
    let i = gpu.Index.x
    y[i] = x[i]
}

let d = gpu.Default()
let x = try await d.Upload([float32(4), 5, 6])
let y = try await d.Upload([float32](repeating: 0, count: 3))
try await copy.Launch(x, y, over: 3)
print(try await y.Download())
// want: [4.0, 5.0, 6.0]
