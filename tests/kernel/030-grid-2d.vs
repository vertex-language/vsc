// A 2D grid: over: (width, height), and gpu.Index.y.
import "gpu"

func table(_ y: gpu.MutableSpan<int32>, width: int) kernel {
    let x = gpu.Index.x
    let row = gpu.Index.y
    y[row * width + x] = int32(row * 10 + x)
}

let y = try await gpu.Default().Upload([int32](repeating: 0, count: 6))
try await table.Launch(y, width: 3, over: (3, 2))
print(try await y.Download())
// want: [0, 1, 2, 10, 11, 12]
