// Helpers calling helpers: the whole chain is compiled for the device.
import "gpu"

func sq(v: int32) -> int32 { return v * v }
func sumSq(a: int32, b: int32) -> int32 { return sq(a) + sq(b) }
func dist2(i: int32) -> int32 { return sumSq(i, i + 1) }

func run(y: gpu.MutableSpan<int32>) kernel {
    let i = gpu.Index.x
    y[i] = dist2(int32(i))
}

let y = try await gpu.Default().Upload([int32](repeating: 0, count: 4))
try await run.Launch(y, over: 4)
print(try await y.Download())
// want: [1, 5, 13, 25]
