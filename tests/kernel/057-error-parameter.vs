// A kernel's parameters are numbers, bools and spans: not a String.
import "gpu"

func named(_ s: string, _ y: gpu.MutableSpan<int32>) kernel {
}
// error: kernel 'named' takes 's' of type 'String'
