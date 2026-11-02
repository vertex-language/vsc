// An ordinary function, compiled for the device because a kernel calls it.
import "gpu"

func clamp(v: int32, lo: int32, hi: int32) -> int32 {
    return v < lo ? lo : (v > hi ? hi : v)
}

func clampAll(y: gpu.MutableSpan<int32>) kernel {
    let i = gpu.Index.x
    y[i] = clamp(y[i], 0, 10)
}

let y = try await gpu.Default().Upload([int32(-5), 3, 42])
try await clampAll.Launch(y, over: 3)
print(try await y.Download())
// want: [0, 3, 10]
