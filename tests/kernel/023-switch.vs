// switch over an integer, with ranges and a default.
import "gpu"

func grade(_ y: gpu.MutableSpan<int32>) kernel {
    let i = gpu.Index.x
    switch y[i] {
    case 0:
        y[i] = 100
    case 1...3:
        y[i] = 200
    case 4, 5:
        y[i] = 300
    default:
        y[i] = -1
    }
}

let y = try await gpu.Default().Upload([int32(0), 2, 5, 9])
try await grade.Launch(y, over: 4)
print(try await y.Download())
// want: [100, 200, 300, -1]
