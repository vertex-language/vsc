// A generic kernel specialized over a struct with no stored properties, for
// its static members alone -- as a quantized block format is. The value
// passed is only its type: nothing reaches the device, and the buffers
// after it bind where they should.
import "gpu"

protocol Format {
    static func Size() -> int
}

struct Narrow: Format {
    static func Size() -> int { return 3 }
}

struct Wide: Format {
    static func Size() -> int { return 5 }
}

func scaled<F: Format>(_ f: F, _ y: gpu.MutableSpan<int32>) kernel {
    y[gpu.Index.x] = int32(F.Size() * gpu.Index.x)
}

let d = gpu.Default()
let y = try await d.CreateBuffer(of: int32.self, count: 4)
try await scaled.Launch(Narrow(), y, over: 4)
print(try await y.Download())
try await scaled.Launch(Wide(), y, over: 4)
print(try await y.Download())
// want: [0, 3, 6, 9]
// want: [0, 5, 10, 15]
