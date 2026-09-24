// gpu.GroupCount: how many workgroups the grid is cut into.
import "gpu"

func count(_ y: gpu.MutableSpan<int32>) kernel {
    y[gpu.Index.x] = int32(gpu.GroupCount.x)
}

let y = try await gpu.Default().Upload([int32](repeating: 0, count: 6))
try await count.Launch(y, over: 6, workgroup: 2)
print(try await y.Download())
// want: [3, 3, 3, 3, 3, 3]
