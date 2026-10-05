// A for loop over a range inside a kernel.
import "gpu"

func triangle(_ y: gpu.MutableSpan<int>) kernel {
    let n = gpu.Index.x
    var sum = 0
    for k in 0...n {
        sum += k
    }
    y[n] = sum
}

let y = try await gpu.Default().Upload([int](repeating: 0, count: 6))
try await triangle.Launch(y, over: 6)
print(try await y.Download())
// want: [0, 1, 3, 6, 10, 15]
