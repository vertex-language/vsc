// guard, the early return a grid kernel usually starts with.
import "gpu"

func square(_ y: gpu.MutableSpan<int32>) kernel {
    let i = gpu.Index.x
    guard i < y.count else { return }
    y[i] = y[i] * y[i]
}

let y = try await gpu.Default().Upload([int32(1), 2, 3, 4])
try await square.Launch(y, over: 64)
print(try await y.Download())
// want: [1, 4, 9, 16]
