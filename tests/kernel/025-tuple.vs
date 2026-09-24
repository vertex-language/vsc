// A tuple returned from a helper and taken apart.
import "gpu"

func divmod(_ a: int32, _ b: int32) -> (int32, int32) {
    return (a / b, a % b)
}

func split(_ q: gpu.MutableSpan<int32>, _ r: gpu.MutableSpan<int32>) kernel {
    let i = gpu.Index.x
    let (a, b) = divmod(int32(i) * 7, 3)
    q[i] = a
    r[i] = b
}

let d = gpu.Default()
let q = try await d.Upload([int32](repeating: 0, count: 4))
let r = try await d.Upload([int32](repeating: 0, count: 4))
try await split.Launch(q, r, over: 4)
print(try await q.Download(), try await r.Download())
// want: [0, 2, 4, 7] [0, 1, 2, 0]
