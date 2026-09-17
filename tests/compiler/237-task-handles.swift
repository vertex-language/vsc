// A Task is a handle to the task it started, and `await t.value` waits for
// that task to finish: one at a time, a copy of one, and handles held in an
// array a function returns. The tasks share nothing, since Swift runs them
// on threads of its own; main counts what it waited for.
func work(_ n: Int) async {
    for _ in 0..<n {
        await Task.yield()
    }
    try? await Task.sleep(nanoseconds: 1_000)
}

func startAll(_ count: Int) -> [Task<Void, Never>] {
    var tasks = [Task<Void, Never>]()
    for i in 0..<count {
        tasks.append(Task { await work(i) })
    }
    return tasks
}

func main() async -> Int32 {
    var waited = 0
    let first = Task { await work(3) }
    await first.value
    waited += 1
    let tasks = startAll(4)
    for t in tasks {
        await t.value
        waited += 2
    }
    let copy: Task<Void, Never> = first
    await copy.value
    waited += 10
    Task { await work(1) }
    return Int32(waited)
}
