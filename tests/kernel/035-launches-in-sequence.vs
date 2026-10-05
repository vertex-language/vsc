// Launch after launch on one buffer: each sees what the last wrote.
import "gpu"

func step(_ y: gpu.MutableSpan<int32>) kernel {
    let i = gpu.Index.x
    y[i] = y[i] * 2 + 1
}

let y = try await gpu.Default().Upload([int32](repeating: 0, count: 3))
for _ in 0..<5 {
    try await step.Launch(y, over: 3)
}
print(try await y.Download())
// want: [31, 31, 31]
