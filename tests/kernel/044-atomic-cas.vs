// Compare-and-exchange: exactly one work-item claims the slot.
import "gpu"

func claim(_ slot: gpu.MutableSpan<int32>, _ wins: gpu.MutableSpan<int32>) kernel {
    let me = int32(gpu.Index.x) + 1
    let was = gpu.Atomic.CompareExchange(slot.Address(0), expected: 0, desired: me)
    if was == 0 {
        _ = gpu.Atomic.Add(wins.Address(0), 1)
    }
}

let d = gpu.Default()
let slot = try await d.Upload([int32(0)])
let wins = try await d.Upload([int32(0)])
try await claim.Launch(slot, wins, over: 500)
let s = try await slot.Download()
print(try await wins.Download(), s[0] >= 1 && s[0] <= 500)
// want: [1] true
