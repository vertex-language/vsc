// saxpy, the first kernel of proposed_vertex_kernel.md.
import "gpu"

func saxpy(_ a: float32, _ x: gpu.Span<float32>, _ y: gpu.MutableSpan<float32>) kernel {
    let i = gpu.Index.x
    if i < y.count {
        y[i] = a * x[i] + y[i]
    }
}

let device = gpu.Default()
let x = try await device.Upload([float32](repeating: 1, count: 1 << 12))
let y = try await device.Upload([float32](repeating: 2, count: 1 << 12))
try await saxpy.Launch(3.0, x, y, over: y.count)
let out = try await y.Download()
print(out[0], out[4095], out.count)
// want: 5.0 5.0 4096
