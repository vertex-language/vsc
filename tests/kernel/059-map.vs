// An element kernel returns one element, and Map applies it over a buffer.
import "gpu"

func square(_ x: float32) kernel -> float32 {
    return x * x
}

let xs = try await gpu.Default().Upload([float32(1), 2, 3, 4])
let ys = try await square.Map(xs)
print(try await ys.Download(), ys.count)
// want: [1.0, 4.0, 9.0, 16.0] 4
