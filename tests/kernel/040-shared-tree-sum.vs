// A tree reduction in shared storage: halving strides, a barrier each step.
import "gpu"

func blockSum(_ x: gpu.Span<int32>, _ out: gpu.MutableSpan<int32>) kernel {
    let partial = gpu.Shared<int32>(count: 8)
    let me = gpu.LocalIndex.x
    partial[me] = x[gpu.Index.x]
    gpu.Barrier()
    var stride = 4
    while stride > 0 {
        if me < stride {
            partial[me] = partial[me] + partial[me + stride]
        }
        gpu.Barrier()
        stride /= 2
    }
    if me == 0 {
        out[gpu.GroupIndex.x] = partial[0]
    }
}

let d = gpu.Default()
var host: [int32] = []
for i in 1...16 { host.append(int32(i)) }
let x = try await d.Upload(host)
let out = try await d.Upload([int32](repeating: 0, count: 2))
try await blockSum.Launch(x, out, over: 16, workgroup: 8)
print(try await out.Download())
// want: [36, 100]
