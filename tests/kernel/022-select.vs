// The conditional operator, and if/else as a value's choice.
import "gpu"

func relu(_ y: gpu.MutableSpan<float32>) kernel {
    let i = gpu.Index.x
    let v = y[i]
    y[i] = v > 0 ? v : 0
}

func sign(_ y: gpu.MutableSpan<int32>) kernel {
    let i = gpu.Index.x
    var s: int32 = 0
    if y[i] > 0 {
        s = 1
    } else if y[i] < 0 {
        s = -1
    }
    y[i] = s
}

let d = gpu.Default()
let a = try await d.Upload([float32(-1.5), 0, 2.5])
let b = try await d.Upload([int32(-9), 0, 4])
try await relu.Launch(a, over: 3)
try await sign.Launch(b, over: 3)
print(try await a.Download(), try await b.Download())
// want: [0.0, 0.0, 2.5] [-1, 0, 1]
