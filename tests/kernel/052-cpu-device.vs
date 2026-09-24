// A kernel launched on buffers of the CPU device runs there, whatever the default is.
import "gpu"

func twice(_ y: gpu.MutableSpan<int32>) kernel {
    let i = gpu.Index.x
    y[i] = y[i] * 2
}

let y = try await gpu.CPU().Upload([int32(1), 2, 3])
try await twice.Launch(y, over: 3)
print(try await y.Download(), y.Device.IsCPU)
// want: [2, 4, 6] true
