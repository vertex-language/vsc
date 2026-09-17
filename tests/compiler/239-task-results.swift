// A Task is a Task of what its operation returns, and `await t.value` is
// that value once the task has finished: an Int, a Bool, a struct of plain
// values, read more than once, and from handles held in an array. The tasks
// share nothing, since Swift runs them on threads of its own.
func compute(_ n: Int) async -> Int {
    for _ in 0..<n {
        await Task.yield()
    }
    return n * n
}

struct Pair {
    var a: Int
    var b: Int
}

func startAll(_ count: Int) -> [Task<Int, Never>] {
    var tasks = [Task<Int, Never>]()
    for i in 1...count {
        tasks.append(Task { await compute(i) })
    }
    return tasks
}

func main() async -> Int32 {
    var total = 0
    for t in startAll(4) {
        total += await t.value
    }
    let p = Task { Pair(a: 3, b: 4) }
    let pair = await p.value
    let again = await p.value
    total += pair.a * pair.b + again.b
    let flag: Task<Bool, Never> = Task { await compute(2) > 3 }
    if await flag.value {
        total += 100
    }
    let plain = Task { await compute(1) }
    await plain.value
    return Int32(total % 251)
}
