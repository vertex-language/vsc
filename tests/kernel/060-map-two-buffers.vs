// Map over two buffers of one count: element i of each.
import "gpu"

func add(a: int32, b: int32) kernel -> int32 {
    return a + b
}

let d = gpu.Default()
let a = try await d.Upload([int32(1), 2, 3])
let b = try await d.Upload([int32(10), 20, 30])
print(try await add.Map(a, b).Download())
// want: [11, 22, 33]
