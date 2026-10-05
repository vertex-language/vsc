// A grid kernel is launched, not called.
import "gpu"

func fill(_ y: gpu.MutableSpan<int32>) kernel {
    y[gpu.Index.x] = 1
}

func host(_ y: gpu.MutableSpan<int32>) {
    fill(y)
}
// error: 'fill' is a kernel: launch it with fill.Launch
