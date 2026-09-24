// Atomic or, and, xor and exchange.
import "gpu"

func bits(_ out: gpu.MutableSpan<int32>) kernel {
    let i = gpu.Index.x
    _ = gpu.Atomic.Or(out.Address(0), int32(1) << int32(i))
    _ = gpu.Atomic.And(out.Address(1), ~(int32(1) << int32(i)))
    _ = gpu.Atomic.Xor(out.Address(2), 1)
    if i == 3 {
        _ = gpu.Atomic.Exchange(out.Address(3), 77)
    }
}

let out = try await gpu.Default().Upload([int32(0), -1, 0, 0])
try await bits.Launch(out, over: 8)
print(try await out.Download())
// want: [255, -256, 0, 77]
