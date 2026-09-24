// A workgroup size chosen by the launch: LocalIndex, GroupIndex, GroupSize.
import "gpu"

func ids(_ local: gpu.MutableSpan<int32>, _ group: gpu.MutableSpan<int32>, _ size: gpu.MutableSpan<int32>) kernel {
    let i = gpu.Index.x
    local[i] = int32(gpu.LocalIndex.x)
    group[i] = int32(gpu.GroupIndex.x)
    size[i] = int32(gpu.GroupSize.x)
}

let d = gpu.Default()
let a = try await d.Upload([int32](repeating: 0, count: 8))
let b = try await d.Upload([int32](repeating: 0, count: 8))
let c = try await d.Upload([int32](repeating: 0, count: 8))
try await ids.Launch(a, b, c, over: 8, workgroup: 4)
print(try await a.Download())
print(try await b.Download())
print(try await c.Download())
// want: [0, 1, 2, 3, 0, 1, 2, 3]
// want: [0, 0, 0, 0, 1, 1, 1, 1]
// want: [4, 4, 4, 4, 4, 4, 4, 4]
