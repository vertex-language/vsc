// Task executor preference (SE-0417): where a task's non-isolated code runs.
final class CountingExecutor: TaskExecutor {
    let name: String
    var jobs = 0
    init(_ name: String) { self.name = name }
    func enqueue(_ job: consuming ExecutorJob) {
        jobs += 1
        job.runSynchronously(on: asUnownedTaskExecutor())
    }
}

func work(_ n: Int) async -> Int {
    var sum = 0
    for i in 0..<n { sum += i }
    await Task.yield()
    return sum
}

let a = CountingExecutor("a")
let r = await withTaskExecutorPreference(a) {
    await work(10)
}
print("result", r, "ran on a:", a.jobs > 0)

let b = CountingExecutor("b")
let t = Task(executorPreference: b) { await work(5) }
print("task", await t.value, "ran on b:", b.jobs > 0)

let c = CountingExecutor("c")
let sums = await withTaskGroup(of: Int.self) { group in
    for i in 1...3 { group.addTask(executorPreference: c) { await work(i * 10) } }
    var total = 0
    for await s in group { total += s }
    return total
}
print("group", sums, "ran on c:", c.jobs > 0)

let d = CountingExecutor("d")
let x = await withTaskExecutorPreference(nil) { await work(4) }
print("nil preference", x, "d untouched:", d.jobs == 0)

let g = await withTaskExecutorPreference(globalConcurrentExecutor) { await work(3) }
print("global", g)

let det = Task.detached(executorPreference: a) { await work(2) }
print("detached", await det.value)
