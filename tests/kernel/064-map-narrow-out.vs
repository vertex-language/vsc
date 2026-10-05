// An element kernel returning a narrow signed type: the sign is kept.
import "gpu"

func dec(_ v: int8) kernel -> int8 {
    return v &- 1
}

let xs = try await gpu.Default().Upload([int8(0), -128, 5])
print(try await dec.Map(xs).Download())
// want: [-1, 127, 4]
