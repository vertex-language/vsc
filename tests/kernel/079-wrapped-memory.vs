// A buffer over host memory another object owns, with no copy: a kernel
// writes through it, and the owner's memory has the change. What a mapped
// weight file is to a model -- its bytes, read in place.
import "gpu"

func twice(_ x: gpu.MutableSpan<float32>) kernel { x[gpu.Index.x] = x[gpu.Index.x] * 2 }

let d = gpu.Default()
let owner = try await d.Upload([float32](repeating: 1.5, count: 8192))
let raw = UnsafeMutableRawPointer(owner._elements)
let wrapped = try d.Wrap(raw, bytes: 8192 * 4, keeping: owner)
let floats = wrapped.View(as: float32.self)
try await twice.Launch(floats, over: 8192)
let seen = try await owner.Download()
print(floats.count, seen[0], seen[8191], try await floats.Slice(from: 100, count: 2).Download())
// want: 8192 3.0 3.0 [3.0, 3.0]
