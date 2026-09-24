// A kernel cannot be async.
import "gpu"

func later(_ y: gpu.MutableSpan<int32>) async kernel {
}
// error: 'later' is a kernel, and a kernel cannot be async
