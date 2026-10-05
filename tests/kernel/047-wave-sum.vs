// Wave.Sum, then one atomic per wave: a total that does not depend on how wide a wave is.
import "gpu"

func total(_ x: gpu.Span<int32>, _ out: gpu.MutableSpan<int32>) kernel {
    let s = gpu.Wave.Sum(x[gpu.Index.x])
    if gpu.Wave.Lane == 0 {
        _ = gpu.Atomic.Add(out.Address(0), s)
    }
}

let d = gpu.Default()
var host: [int32] = []
for i in 1...128 { host.append(int32(i)) }
let x = try await d.Upload(host)
let out = try await d.Upload([int32(0)])
try await total.Launch(x, out, over: 128, workgroup: 64)
print(try await out.Download())
// want: [8256]
