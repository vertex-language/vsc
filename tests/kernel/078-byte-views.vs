// A buffer filled from host memory by pointer, and a slice of its bytes
// viewed as float32s without a copy: a kernel writes through the view,
// and the bytes' own view sees it.
import "gpu"

func twice(_ x: gpu.MutableSpan<float32>) kernel { x[gpu.Index.x] = x[gpu.Index.x] * 2 }

let d = gpu.Default()
var bytes: [uint8] = [0, 0, 0, 0]
for v in [float32(1.5), -3] { let b = v.bitPattern; bytes += [uint8(truncatingIfNeeded: b), uint8(truncatingIfNeeded: b >> 8), uint8(truncatingIfNeeded: b >> 16), uint8(truncatingIfNeeded: b >> 24)] }
let host = try await d.Upload(bytes)
let raw = try await d.Upload(from: UnsafePointer<uint8>(host._elements), count: 12)
let f = raw.Slice(from: 4, count: 8).View(as: float32.self)
try await twice.Launch(f, over: 2)
print(f.count, try await f.Download(), try await raw.View(as: float32.self).Download())
// want: 2 [3.0, -6.0] [0.0, 3.0, -6.0]
