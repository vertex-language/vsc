// Local variables, reassigned, and arithmetic across them.
import "gpu"

func poly(_ y: gpu.MutableSpan<int32>) kernel {
    let i = int32(gpu.Index.x)
    var acc: int32 = 1
    acc = acc * i + 2
    acc = acc * i + 3
    y[gpu.Index.x] = acc
}

let y = try await gpu.Default().Upload([int32](repeating: 0, count: 4))
try await poly.Launch(y, over: 4)
print(try await y.Download())
// want: [3, 6, 11, 18]
