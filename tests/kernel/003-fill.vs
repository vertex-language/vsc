// Every work-item writes its own element, found by gpu.Index.x.
import "gpu"

func fill(_ y: gpu.MutableSpan<int32>) kernel {
    y[gpu.Index.x] = 1
}

let y = try await gpu.Default().Upload([int32](repeating: 0, count: 6))
try await fill.Launch(y, over: 6)
print(try await y.Download())
// want: [1, 1, 1, 1, 1, 1]
