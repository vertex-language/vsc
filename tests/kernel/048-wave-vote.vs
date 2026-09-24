// Wave votes: Any and All over a condition every lane agrees on.
import "gpu"

func vote(_ out: gpu.MutableSpan<int32>) kernel {
    let i = gpu.Index.x
    let even = i % 2 == 0
    let a = gpu.Wave.Any(i >= 0)
    let b = gpu.Wave.All(i < 1000)
    let c = gpu.Wave.All(i < 0)
    out[i] = (a ? 1 : 0) + (b ? 2 : 0) + (c ? 4 : 0) + (even ? 0 : 0)
}

let out = try await gpu.Default().Upload([int32](repeating: 0, count: 32))
try await vote.Launch(out, over: 32, workgroup: 32)
let got = try await out.Download()
print(got[0], got[31], got.reduce(0, +))
// want: 3 3 96
