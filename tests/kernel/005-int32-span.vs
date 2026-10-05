// Reading and writing int32, negative values included.
import "gpu"

func negate(_ x: gpu.Span<int32>, _ y: gpu.MutableSpan<int32>) kernel {
    let i = gpu.Index.x
    y[i] = -x[i]
}

let d = gpu.Default()
let x = try await d.Upload([int32(1), -2, 300000, -2147483647])
let y = try await d.Upload([int32](repeating: 0, count: 4))
try await negate.Launch(x, y, over: 4)
print(try await y.Download())
// want: [-1, 2, -300000, 2147483647]
