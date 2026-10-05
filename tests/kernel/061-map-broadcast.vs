// A plain value where a buffer could go is the same for every element.
import "gpu"

func lerp(_ a: float32, _ b: float32, _ t: float32) kernel -> float32 {
    return a + (b - a) * t
}

let d = gpu.Default()
let from = try await d.Upload([float32(0), 10, 20])
let to = try await d.Upload([float32(4), 14, 40])
print(try await lerp.Map(from, to, 0.25).Download())
print(try await lerp.Map(from, 100, 0.5).Download())
// want: [1.0, 11.0, 25.0]
// want: [50.0, 55.0, 60.0]
