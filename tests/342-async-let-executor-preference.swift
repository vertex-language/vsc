// async let inherits the task executor preference it is written under: the
// child's jobs, and not only the parent's resumption, run on the executor.
final class CountingExecutor: TaskExecutor {
    var jobs = 0
    func enqueue(_ job: consuming ExecutorJob) {
        jobs += 1
        job.runSynchronously(on: asUnownedTaskExecutor())
    }
}
func work(_ n: Int) async -> Int {
    await Task.yield()
    await Task.yield()
    return n * 2
}
let e = CountingExecutor()
let r = await withTaskExecutorPreference(e) {
    let start = e.jobs
    async let x = work(20)
    let v = await x + 2
    return (v, e.jobs - start > 1)
}
print(r.0, "child ran on e:", r.1)
