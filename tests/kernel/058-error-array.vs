// An Array lives on a heap a device does not have.
import "gpu"

func sums(_ y: gpu.MutableSpan<int32>) kernel {
    let xs: [int32] = [1, 2, 3]
    y[gpu.Index.x] = xs[0]
}
print("never built")
// error: kernel 'sums' cannot run on a device
