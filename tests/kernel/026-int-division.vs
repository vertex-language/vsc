// Integer division and remainder, signed, truncating toward zero.
import "gpu"

func divs(_ y: gpu.MutableSpan<int32>) kernel {
    let i = gpu.Index.x
    let v = y[i]
    y[i] = v / 4 * 10 + v % 4
}

let y = try await gpu.Default().Upload([int32(17), -17, 3, -3])
try await divs.Launch(y, over: 4)
print(try await y.Download())
// want: [41, -41, 3, -3]
