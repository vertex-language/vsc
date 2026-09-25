// int64 and uint64 parameters, passed by value as int and uint are.
import "gpu"

func offsets(_ y: gpu.MutableSpan<uint64>, _ base: uint64, _ step: int64) kernel {
    let i = gpu.Index.x
    y[i] = base &+ uint64(bitPattern: step * int64(i))
}

let y = try await gpu.Default().CreateBuffer(of: uint64.self, count: 3)
try await offsets.Launch(y, 0xFFFFFFFF00000000, -2, over: 3)
print(try await y.Download())
// want: [18446744069414584320, 18446744069414584318, 18446744069414584316]
