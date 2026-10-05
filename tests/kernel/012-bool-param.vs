// A bool passed by value chooses what the kernel does.
import "gpu"

func pick(_ on: bool, _ y: gpu.MutableSpan<int32>) kernel {
    y[gpu.Index.x] = on ? 1 : 2
}

let d = gpu.Default()
let a = try await d.Upload([int32](repeating: 0, count: 2))
let b = try await d.Upload([int32](repeating: 0, count: 2))
try await pick.Launch(true, a, over: 2)
try await pick.Launch(false, b, over: 2)
print(try await a.Download(), try await b.Download())
// want: [1, 1] [2, 2]
