// Shared storage and a barrier: a workgroup reverses its own block.
import "gpu"

func reverseBlocks(_ y: gpu.MutableSpan<int32>) kernel {
    let tile = gpu.Shared<int32>(count: 4)
    let me = gpu.LocalIndex.x
    tile[me] = y[gpu.Index.x]
    gpu.Barrier()
    y[gpu.Index.x] = tile[3 - me]
}

let y = try await gpu.Default().Upload([int32(1), 2, 3, 4, 5, 6, 7, 8])
try await reverseBlocks.Launch(y, over: 8, workgroup: 4)
print(try await y.Download())
// want: [4, 3, 2, 1, 8, 7, 6, 5]
