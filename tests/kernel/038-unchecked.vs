// Unchecked reads and writes: no bounds check, for a kernel that proved them.
import "gpu"

func rev(_ x: gpu.Span<int32>, _ y: gpu.MutableSpan<int32>) kernel {
    let i = gpu.Index.x
    y.SetUnchecked(i, x.Unchecked(x.count - 1 - i))
}

let d = gpu.Default()
let x = try await d.Upload([int32(1), 2, 3, 4])
let y = try await d.Upload([int32](repeating: 0, count: 4))
try await rev.Launch(x, y, over: 4)
print(try await y.Download())
// want: [4, 3, 2, 1]
