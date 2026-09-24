// A while loop whose trip count differs per work-item.
import "gpu"

func collatz(_ y: gpu.MutableSpan<int32>) kernel {
    var n = y[gpu.Index.x]
    var steps: int32 = 0
    while n != 1 {
        n = n % 2 == 0 ? n / 2 : 3 * n + 1
        steps += 1
    }
    y[gpu.Index.x] = steps
}

let y = try await gpu.Default().Upload([int32(1), 2, 3, 6, 7, 27])
try await collatz.Launch(y, over: 6)
print(try await y.Download())
// want: [0, 1, 7, 8, 16, 111]
