// A grid larger than the data, and the guard that keeps it in bounds.
import "gpu"

func ones(_ y: gpu.MutableSpan<int32>) kernel {
    let i = gpu.Index.x
    if i >= y.count {
        return
    }
    y[i] = 1
}

let y = try await gpu.Default().Upload([int32](repeating: 0, count: 5))
try await ones.Launch(y, over: 100)
print(try await y.Download())
// want: [1, 1, 1, 1, 1]
