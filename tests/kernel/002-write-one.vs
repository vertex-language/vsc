// One work-item writes one element.
import "gpu"

func seven(_ y: gpu.MutableSpan<int32>) kernel {
    y[0] = 7
}

let y = try await gpu.Default().Upload([int32](repeating: 0, count: 3))
try await seven.Launch(y, over: 1)
print(try await y.Download())
// want: [7, 0, 0]
