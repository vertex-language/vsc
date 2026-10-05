// int is 64 bits on the device too.
import "gpu"

func big(_ y: gpu.MutableSpan<int>) kernel {
    let i = gpu.Index.x
    y[i] = y[i] * 1000000
}

let y = try await gpu.Default().Upload([1, 2, 5000000])
try await big.Launch(y, over: 3)
print(try await y.Download())
// want: [1000000, 2000000, 5000000000000]
