// A launch is typed against the kernel: a buffer of the wrong element is refused.
import "gpu"

func fill(_ y: gpu.MutableSpan<int32>) kernel {
    y[gpu.Index.x] = 1
}

let y = try await gpu.Default().Upload([float32(0)])
try await fill.Launch(y, over: 1)
// error: cannot launch fill with 'Buffer<Float>' for 'y'
