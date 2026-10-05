// Generic kernels: built for each type they are launched with, directly
// and from a generic function whose own type parameter the launch passes.
// A protocol of the program's own, conformed to by `extension float32`,
// gives them what they need beyond arithmetic.
import "gpu"

protocol Bounded: Numeric, Comparable {
    static func Top() -> Self
}

extension float32: Bounded {
    static func Top() -> float32 { return 100 }
}

extension int32: Bounded {
    static func Top() -> int32 { return 7 }
}

func clampScale<T: Bounded>(_ y: gpu.MutableSpan<T>, _ k: T) kernel {
    let i = gpu.Index.x
    let v = y[i] * k
    y[i] = v > T.Top() ? T.Top() : v
}

func apply<T: Bounded>(_ b: gpu.Buffer<T>, _ k: T) async throws {
    try await clampScale.Launch(b, k, over: b.count)
}

let d = gpu.Default()
let f = try await d.Upload([float32(1), 30, 60])
try await clampScale.Launch(f, 2, over: 3)
let n = try await d.Upload([int32(1), 2, 5])
try await apply(n, 3)
print(try await f.Download(), try await n.Download())
// want: [2.0, 60.0, 100.0] [3, 6, 7]
