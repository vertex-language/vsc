// A 3D grid: over: (w, h, d), and gpu.Index.z.
import "gpu"

func cube(_ y: gpu.MutableSpan<int32>) kernel {
    let i = (gpu.Index.z * 2 + gpu.Index.y) * 2 + gpu.Index.x
    y[i] = int32(gpu.Index.z * 100 + gpu.Index.y * 10 + gpu.Index.x)
}

let y = try await gpu.Default().Upload([int32](repeating: 0, count: 8))
try await cube.Launch(y, over: (2, 2, 2))
print(try await y.Download())
// want: [0, 1, 10, 11, 100, 101, 110, 111]
