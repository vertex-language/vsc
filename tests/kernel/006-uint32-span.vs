// uint32, including values past int32's range.
import "gpu"

func double(_ y: gpu.MutableSpan<uint32>) kernel {
    let i = gpu.Index.x
    y[i] = y[i] &* 2
}

let y = try await gpu.Default().Upload([uint32(1), 7, 3000000000])
try await double.Launch(y, over: 3)
print(try await y.Download())
// want: [2, 14, 1705032704]
