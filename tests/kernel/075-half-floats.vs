// float16 and bfloat16 in kernels: spans of them, parameters of them, and
// an element kernel over them, computed in half precision on every device
// -- natively where the device has half instructions, in f32 and rounded
// once where it does not, which gives the same bits.
import "gpu"

func axpy(_ y: gpu.MutableSpan<float16>, _ x: gpu.Span<float16>, _ a: float16) kernel {
    let i = gpu.Index.x
    y[i] = a * x[i] + y[i]
}

func widen(_ x: bfloat16, _ s: float32) kernel -> float32 {
    return float32(x) * s
}

func narrow(_ x: float32) kernel -> bfloat16 {
    return bfloat16(x)
}

let d = gpu.Default()
let x = try await d.Upload([float16(1), 2, 0.1, 1000])
let y = try await d.Upload([float16(0.5), 0.25, 3, -1])
try await axpy.Launch(y, x, 1.5, over: 4)
print(try await y.Download())
let b = try await d.Upload([bfloat16(1.5), 3.14159, -2])
print(try await widen.Map(b, 2).Download())
let f = try await d.Upload([float32(0.1), 1e30, -7.75])
print(try await narrow.Map(f).Download())
// want: [2.0, 3.25, 3.15, 1499.0]
// want: [3.0, 6.28125, -4.0]
// want: [0.1, 1e+30, -7.75]
