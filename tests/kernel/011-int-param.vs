// An int passed by value, and a labelled parameter.
import "gpu"

func fill(_ y: gpu.MutableSpan<int>, with v: int) kernel {
    y[gpu.Index.x] = v
}

let y = try await gpu.Default().Upload([0, 0, 0])
try await fill.Launch(y, with: 123456789012, over: 3)
print(try await y.Download())
// want: [123456789012, 123456789012, 123456789012]
