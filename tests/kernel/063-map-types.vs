// Map between element types: bytes in, wider integers and bools out.
import "gpu"

func widen(_ b: uint8) kernel -> int32 {
    return int32(b) * 1000
}

func isOdd(_ v: int32) kernel -> bool {
    return v % 2 != 0
}

let d = gpu.Default()
let bytes = try await d.Upload([uint8(0), 1, 200, 255])
let wide = try await widen.Map(bytes)
print(try await wide.Download())
print(try await isOdd.Map(try await d.Upload([int32(1), 2, -3])).Download())
// want: [0, 1000, 200000, 255000]
// want: [true, false, true]
