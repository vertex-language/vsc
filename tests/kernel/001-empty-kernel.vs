// A kernel that does nothing, launched over one work-item.
import "gpu"

func nothing(_ y: gpu.MutableSpan<int32>) kernel {
}

let y = try await gpu.Default().Upload([int32(3)])
try await nothing.Launch(y, over: 1)
print(try await y.Download())
// want: [3]
