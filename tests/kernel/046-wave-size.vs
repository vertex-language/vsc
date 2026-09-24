// A wave is at least one work-item wide, and a lane is inside it.
import "gpu"

func waves(_ out: gpu.MutableSpan<int32>) kernel {
    let ok = gpu.Wave.Size >= 1 && gpu.Wave.Lane >= 0 && gpu.Wave.Lane < gpu.Wave.Size
    out[gpu.Index.x] = ok ? 1 : 0
}

let out = try await gpu.Default().Upload([int32](repeating: 0, count: 64))
try await waves.Launch(out, over: 64)
let got = try await out.Download()
print(got.reduce(0, +))
// want: 64
