// The index as a value: each element is where it is.
import "gpu"

func iota(_ y: gpu.MutableSpan<float32>) kernel {
    let i = gpu.Index.x
    y[i] = float32(i)
}

let y = try await gpu.Default().Upload([float32](repeating: 0, count: 5))
try await iota.Launch(y, over: 5)
print(try await y.Download())
// want: [0.0, 1.0, 2.0, 3.0, 4.0]
