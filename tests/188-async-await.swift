// async functions awaited from top-level code, in order.
func square(_ x: Int) async -> Int {
    await Task.yield()
    return x * x
}
func sumOfSquares(_ xs: [Int]) async -> Int {
    var total = 0
    for x in xs { total += await square(x) }
    return total
}
print(await square(7))
print(await sumOfSquares([1, 2, 3, 4]))
