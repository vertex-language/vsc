// An atomic add: every work-item counts into one element.
import "gpu"

func count(_ n: gpu.MutableSpan<int32>) kernel {
    _ = gpu.Atomic.Add(n.Address(0), 1)
}

let n = try await gpu.Default().Upload([int32(0)])
try await count.Launch(n, over: 1000)
print(try await n.Download())
// want: [1000]
