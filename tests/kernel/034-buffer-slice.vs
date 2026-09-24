// A slice of a buffer, launched on: the kernel sees only the view.
import "gpu"

func mark(_ y: gpu.MutableSpan<int32>) kernel {
    let i = gpu.Index.x
    y[i] = int32(100 + i + y.count)
}

let all = try await gpu.Default().Upload([int32](repeating: 0, count: 6))
try await mark.Launch(all.Slice(from: 2, count: 3), over: 3)
print(try await all.Download())
// want: [0, 0, 103, 104, 105, 0]
