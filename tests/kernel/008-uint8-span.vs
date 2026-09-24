// Bytes: a span of uint8, one work-item per byte.
import "gpu"

func invert(_ y: gpu.MutableSpan<uint8>) kernel {
    let i = gpu.Index.x
    y[i] = 255 - y[i]
}

let y = try await gpu.Default().Upload([uint8(0), 1, 128, 255])
try await invert.Launch(y, over: 4)
print(try await y.Download())
// want: [255, 254, 127, 0]
