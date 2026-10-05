// An int32 passed by value.
import "gpu"

func offset(_ k: int32, _ y: gpu.MutableSpan<int32>) kernel {
    let i = gpu.Index.x
    y[i] = y[i] + k
}

let y = try await gpu.Default().Upload([int32(10), 20])
try await offset.Launch(-3, y, over: 2)
print(try await y.Download())
// want: [7, 17]
