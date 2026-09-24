// One kernel writing two buffers.
import "gpu"

func minmax(_ x: gpu.Span<int32>, _ lo: gpu.MutableSpan<int32>, _ hi: gpu.MutableSpan<int32>) kernel {
    let i = gpu.Index.x
    let a = x[2 * i]
    let b = x[2 * i + 1]
    lo[i] = a < b ? a : b
    hi[i] = a < b ? b : a
}

let d = gpu.Default()
let x = try await d.Upload([int32(5), 1, -2, 8, 7, 7])
let lo = try await d.Upload([int32](repeating: 0, count: 3))
let hi = try await d.Upload([int32](repeating: 0, count: 3))
try await minmax.Launch(x, lo, hi, over: 3)
print(try await lo.Download(), try await hi.Download())
// want: [1, -2, 7] [5, 8, 7]
