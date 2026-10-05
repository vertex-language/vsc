// The host side alone: create, fill, copy, slice and download buffers.
import "gpu"

let d = gpu.Default()
let a = try d.CreateBuffer(of: int32.self, count: 5)
try await a.Fill(9)
let b = try await d.Upload([int32(1), 2, 3, 4, 5])
try await a.Slice(from: 1, count: 3).Copy(from: b.Slice(from: 2, count: 3))
print(try await a.Download(), a.count)
try await b.Upload([int32(5), 4, 3, 2, 1])
print(try await b.Download())
// want: [9, 3, 4, 5, 9] 5
// want: [5, 4, 3, 2, 1]
