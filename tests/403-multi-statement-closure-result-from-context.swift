// A multi-statement closure passed to a generic function takes its result
// type from what the call is wanted as, not from its first return.
final class Lock {
    func withLock<R>(_ body: () throws -> R) rethrows -> R { try body() }
}
struct Q { let n: Int }
let lock = Lock()
var queue: [Q] = [Q(n: 4)]
func take() -> Q? {
    let q: Q? = lock.withLock {
        guard !queue.isEmpty else { return nil }
        return queue.removeFirst()
    }
    return q
}
print(take()?.n ?? -1, take()?.n ?? -1)
func status() -> (UInt16, UInt32) {
    return lock.withLock {
        let code: UInt16 = 200
        return (code, 0)
    }
}
let (code, len) = status()
print(code, len, type(of: len))
