// A program: matrix multiply over a 2D grid, one work-item per output element.
import "gpu"

func matmul(_ a: gpu.Span<float32>, _ b: gpu.Span<float32>, _ c: gpu.MutableSpan<float32>, n: int) kernel {
    let col = gpu.Index.x
    let row = gpu.Index.y
    if row >= n || col >= n {
        return
    }
    var sum: float32 = 0
    for k in 0..<n {
        sum += a[row * n + k] * b[k * n + col]
    }
    c[row * n + col] = sum
}

let n = 4
let d = gpu.Default()
var ha: [float32] = []
var hb: [float32] = []
for i in 0..<(n * n) {
    ha.append(float32(i % 5))
    hb.append(i % (n + 1) == 0 ? 1 : 0)
}
let a = try await d.Upload(ha)
let b = try await d.Upload(hb)
let c = try await d.CreateBuffer(of: float32.self, count: n * n)
try await matmul.Launch(a, b, c, n: n, over: (n, n))
print(try await c.Download() == ha)
// want: true
