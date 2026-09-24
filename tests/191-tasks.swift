// An unstructured Task, its value, and throwing and cancellation.
struct Oops: Error {}
let t = Task { () -> String in
    await Task.yield()
    return "done"
}
print(await t.value)
let failing = Task { () throws -> Int in throw Oops() }
do {
    _ = try await failing.value
} catch {
    print("task threw", error is Oops)
}
let cancelled = Task { () -> Bool in
    try? await Task.sleep(nanoseconds: 1_000_000_000)
    return Task.isCancelled
}
cancelled.cancel()
print(await cancelled.value)
