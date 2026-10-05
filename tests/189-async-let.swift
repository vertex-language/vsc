// async let starts child tasks that run alongside, joined when awaited.
func work(_ id: Int) async -> Int {
    try? await Task.sleep(nanoseconds: UInt64(10 - id) * 1_000_000)
    return id * 100
}
async let a = work(1)
async let b = work(2)
async let c = work(3)
let results = await [a, b, c]
print(results, results.reduce(0, +))
