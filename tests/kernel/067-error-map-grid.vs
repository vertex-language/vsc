// Map is for element kernels: a grid kernel returns nothing to map.
import "gpu"

func fill(_ y: gpu.MutableSpan<int32>) kernel {
    y[gpu.Index.x] = 1
}

let y = try await gpu.Default().Upload([int32(0)])
_ = try await fill.Map(y)
// error: 'fill' is a grid kernel, which returns nothing to map
