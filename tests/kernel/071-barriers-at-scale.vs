// Many full groups, each passing many barriers: 64 groups of 1024, and 32
// rounds of a shared rotation. On the CPU device each group runs its
// work-items as fibers on one thread, so this is fast there too.
import "gpu"

func rotate(_ y: gpu.MutableSpan<int32>) kernel {
    let tile = gpu.Shared<int32>(count: 1024)
    let me = gpu.LocalIndex.x
    var v = int32(gpu.Index.x)
    var round = 0
    while round < 32 {
        tile[me] = v
        gpu.Barrier()
        v = tile[(me + 1) % 1024]
        gpu.Barrier()
        round += 1
    }
    y[gpu.Index.x] = v
}

let n = 64 * 1024
let y = try await gpu.Default().CreateBuffer(of: int32.self, count: n)
try await rotate.Launch(y, over: n, workgroup: 1024)
let got = try await y.Download()
var ok = true
for i in 0..<n {
    let g = i / 1024 * 1024
    if got[i] != int32(g + (i - g + 32) % 1024) { ok = false }
}
print(ok)
// want: true
