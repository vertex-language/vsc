// A task's operation may return a value that owns something -- a String,
// a struct holding one, an array of them -- and `await t.value` is a copy
// of it, as often as it is read. The tasks share nothing, since Swift runs
// them on threads of its own.
struct Named {
    var name: String
    var n: Int
}

func make(_ i: Int) async -> Named {
    await Task.yield()
    return Named(name: "task number \(i) with a long name", n: i)
}

func words() async -> [String] {
    await Task.yield()
    return ["a", "bb", "ccc"]
}

func greeting() async -> String {
    return "a string long enough to live on the heap"
}

func main() async -> Int32 {
    var total = 0
    var tasks = [Task<Named, Never>]()
    for i in 1...3 {
        tasks.append(Task { await make(i) })
    }
    for t in tasks {
        let v = await t.value
        total += v.n + v.name.count
    }
    let w = Task { await words() }
    for s in await w.value {
        total += s.count
    }
    let again = await w.value
    total += again.count
    let g: Task<String, Never> = Task { await greeting() }
    total += await g.value.count
    return Int32(total % 251)
}
