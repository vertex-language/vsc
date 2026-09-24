// A struct of the program's own, as a local value on the device.
import "gpu"

struct Vec2 {
    var x: float32
    var y: float32
    func dot(_ o: Vec2) -> float32 { return x * o.x + y * o.y }
}

func dots(_ out: gpu.MutableSpan<float32>) kernel {
    let i = float32(gpu.Index.x)
    let a = Vec2(x: i, y: 1)
    var b = Vec2(x: 2, y: i)
    b.y += 1
    out[gpu.Index.x] = a.dot(b)
}

let out = try await gpu.Default().Upload([float32](repeating: 0, count: 3))
try await dots.Launch(out, over: 3)
print(try await out.Download())
// want: [1.0, 4.0, 7.0]
