// Wave.Max and Wave.Min, folded into atomics per wave.
import "gpu"

func spread(_ x: gpu.Span<int32>, _ out: gpu.MutableSpan<int32>) kernel {
    let v = x[gpu.Index.x]
    let hi = gpu.Wave.Max(v)
    let lo = gpu.Wave.Min(v)
    if gpu.Wave.Lane == 0 {
        _ = gpu.Atomic.Max(out.Address(0), hi)
        _ = gpu.Atomic.Min(out.Address(1), lo)
    }
}

let d = gpu.Default()
var host: [int32] = []
for i in 0..<96 { host.append(int32((i * 29) % 97)) }
let x = try await d.Upload(host)
let out = try await d.Upload([int32(-1), 1000])
try await spread.Launch(x, out, over: 96, workgroup: 32)
print(try await out.Download())
// want: [96, 0]
