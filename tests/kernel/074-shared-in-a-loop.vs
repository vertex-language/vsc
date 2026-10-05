// A gpu.Shared asked for inside a loop is one allocation, used again every
// time round, on every device: here 100 rounds of 4 KB, which would be
// 400 KB if each round took storage of its own.
import "gpu"

func rounds(_ y: gpu.MutableSpan<int32>) kernel {
    let me = gpu.LocalIndex.x
    var total: int32 = 0
    var r = 0
    while r < 100 {
        let tile = gpu.Shared<int32>(count: 1024)
        tile[me] = int32(r)
        gpu.Barrier()
        total += tile[(me + 1) % 64]
        gpu.Barrier()
        r += 1
    }
    y[gpu.Index.x] = total
}

let y = try await gpu.Default().CreateBuffer(of: int32.self, count: 64)
try await rounds.Launch(y, over: 64, workgroup: 64)
print(try await y.Download()[0], try await y.Download()[63])
// want: 4950 4950
