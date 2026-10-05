// An ordinary function, compiled for the device because a kernel calls it.
import "gpu"

func clamp(_ v: int32, _ lo: int32, _ hi: int32) -> int32 {
    return v < lo ? lo : (v > hi ? hi : v)
}

func clampAll(_ y: gpu.MutableSpan<int32>) kernel {
    let i = gpu.Index.x
    y[i] = clamp(y[i], 0, 10)
}

let y = try await gpu.Default().Upload([int32(-5), 3, 42])
try await clampAll.Launch(y, over: 3)
print(try await y.Download())
// want: [0, 3, 10]
