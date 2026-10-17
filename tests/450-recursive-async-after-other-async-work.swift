// A recursive async function that copies arrays, called after other async
// work has run.
func fetch(_ n: Int) async -> [UInt8] { [UInt8](repeating: UInt8(n), count: 4096) }
func copyTree(_ depth: Int, _ block: [UInt8]) async -> Int {
    if depth == 0 { return block.reduce(0) { $0 + Int($1) } }
    var next = block
    next[0] &+= 1
    let a = await copyTree(depth - 1, next)
    let b = await copyTree(depth - 1, Array(next.prefix(2048)))
    return a + b
}
let text = await fetch(1)
print(text.count)
print(await copyTree(8, text))
