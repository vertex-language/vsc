// Shifts, masks and bitwise operators on uint32.
import "gpu"

func bits(_ y: gpu.MutableSpan<uint32>) kernel {
    let i = gpu.Index.x
    let v = y[i]
    y[i] = ((v << 4) | (v >> 28)) ^ 0xFF & ~uint32(0)
}

let y = try await gpu.Default().Upload([uint32(1), 0x80000000, 0x12345678])
try await bits.Launch(y, over: 3)
print(try await y.Download())
// want: [239, 247, 591751038]
