// An async call with more arguments than fit in registers.
func sum(_ a: Int, _ b: Int, _ c: Int, _ d: Int, _ e: Int,
         _ f: Int, _ g: Int, _ h: Int, _ i: Int, _ j: Int) async -> Int {
    a + b + c + d + e + f + g + h + i + j
}
func run() async -> Int { await sum(1, 2, 3, 4, 5, 6, 7, 8, 9, 10) }
print(await run())
print(await sum(10, 20, 30, 40, 50, 60, 70, 80, 90, 100))
