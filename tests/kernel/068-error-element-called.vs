// An element kernel is applied with Map on the host, not called.
import "gpu"

func square(_ x: float32) kernel -> float32 {
    return x * x
}

print(square(3))
// error: 'square' is an element kernel: apply it over buffers with square.Map
