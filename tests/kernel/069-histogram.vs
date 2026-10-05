// A program: a histogram of bytes, with atomics into shared storage then into the result.
import "gpu"

func histogram(_ data: gpu.Span<uint8>, _ bins: gpu.MutableSpan<int32>) kernel {
    let local = gpu.Shared<int32>(count: 4)
    let me = gpu.LocalIndex.x
    if me < 4 {
        local[me] = 0
    }
    gpu.Barrier()
    let i = gpu.Index.x
    if i < data.count {
        _ = gpu.Atomic.Add(local.Address(int(data[i]) / 64), 1)
    }
    gpu.Barrier()
    if me < 4 {
        _ = gpu.Atomic.Add(bins.Address(me), local[me])
    }
}

let d = gpu.Default()
var bytes: [uint8] = []
for i in 0..<1000 { bytes.append(uint8((i * 7) % 256)) }
let data = try await d.Upload(bytes)
let bins = try await d.Upload([int32](repeating: 0, count: 4))
try await histogram.Launch(data, bins, over: 1024, workgroup: 64)
let got = try await bins.Download()
print(got, got.reduce(0, +))
// want: [256, 250, 247, 247] 1000
