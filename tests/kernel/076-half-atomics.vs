// gpu.Atomic.Add of a float16 and of a bfloat16: a compare-and-swap on the
// 32-bit word holding the half, so neighbours sharing a word both land.
import "gpu"

func add(_ a: gpu.MutableSpan<float16>, _ b: gpu.MutableSpan<bfloat16>, _ n: int) kernel {
    let i = gpu.Index.x
    _ = gpu.Atomic.Add(a.Address(i % n), 0.5)
    _ = gpu.Atomic.Add(b.Address(i % n), 1)
}

let d = gpu.Default()
let a = try await d.Upload([float16](repeating: 0, count: 5))
let b = try await d.Upload([bfloat16](repeating: 0, count: 5))
try await add.Launch(a, b, 5, over: 1000)
print(try await a.Download())
print(try await b.Download())
// want: [100.0, 100.0, 100.0, 100.0, 100.0]
// want: [200.0, 200.0, 200.0, 200.0, 200.0]
