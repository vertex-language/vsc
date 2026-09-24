// Map into a buffer that already exists, instead of a new one.
import "gpu"

func negate(_ x: int) kernel -> int {
    return -x
}

let d = gpu.Default()
let xs = try await d.Upload([1, -2, 3])
let out = try d.CreateBuffer(of: int.self, count: 3)
try await negate.Map(xs, into: out)
print(try await out.Download())
// want: [-1, 2, -3]
