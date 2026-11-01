// Two gpu.Shared in one kernel are two allocations, on every device: on
// the CPU, where gpu.Shared's initializer is inlined, both once came from
// one place and were the same storage, so b's writes landed in a.
import "gpu"

func two(_ y: gpu.MutableSpan<int32>) kernel {
    let a = gpu.Shared<int32>(count: 64)
    let b = gpu.Shared<int32>(count: 64)
    let me = gpu.LocalIndex.x
    a[me] = 1
    b[me] = 2
    gpu.Barrier()
    y[gpu.Index.x] = a[(me + 1) % 64] * 10 + b[(me + 1) % 64]
}

let y = try await gpu.Default().CreateBuffer(of: int32.self, count: 64)
try await two.Launch(y, over: 64, workgroup: 64)
print(try await y.Download()[0], try await y.Download()[63])
// want: 12 12
