// A span knows its count; one work-item walks all of it.
import "gpu"

func total(_ x: gpu.Span<int32>, _ out: gpu.MutableSpan<int32>) kernel {
    var s: int32 = 0
    var i = 0
    while i < x.count {
        s += x[i]
        i += 1
    }
    out[0] = s
}

let d = gpu.Default()
let x = try await d.Upload([int32](repeating: 3, count: 100))
let out = try await d.Upload([int32(0)])
try await total.Launch(x, out, over: 1)
print(try await out.Download())
// want: [300]
