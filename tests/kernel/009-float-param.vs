// A float32 passed by value: the same for every work-item.
import "gpu"

func scale(_ a: float32, _ y: gpu.MutableSpan<float32>) kernel {
    let i = gpu.Index.x
    y[i] = y[i] * a
}

let y = try await gpu.Default().Upload([float32(1), 2, 3])
try await scale.Launch(2.5, y, over: 3)
print(try await y.Download())
// want: [2.5, 5.0, 7.5]
