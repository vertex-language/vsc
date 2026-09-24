// Atomic min and max over every element.
import "gpu"

func extremes(_ x: gpu.Span<int32>, _ out: gpu.MutableSpan<int32>) kernel {
    let v = x[gpu.Index.x]
    _ = gpu.Atomic.Min(out.Address(0), v)
    _ = gpu.Atomic.Max(out.Address(1), v)
}

let d = gpu.Default()
var host: [int32] = []
for i in 0..<64 { host.append(int32((i * 37) % 64) - 20) }
let x = try await d.Upload(host)
let out = try await d.Upload([int32(1000), -1000])
try await extremes.Launch(x, out, over: 64)
print(try await out.Download())
// want: [-20, 43]
