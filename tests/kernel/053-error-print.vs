// A kernel cannot print: refused where it is compiled, with the chain.
import "gpu"

func log(_ v: int32) {
    print(v)
}

func noisy(_ y: gpu.MutableSpan<int32>) kernel {
    log(y[gpu.Index.x])
}
print("never built")
// error: kernel 'noisy' cannot run on a device: noisy calls log
