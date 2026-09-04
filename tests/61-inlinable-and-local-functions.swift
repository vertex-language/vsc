// §7 / §6.4 Inlinable Functions with Local Nested Functions

@usableFromInline
internal func computeHash(seed: Int, value: Int) -> Int {
    return (seed &* 31) &+ value
}

@inlinable
public func fastProcessBatch<T: Sequence>(_ items: T) -> Int where T.Element == Int {
    @inline(__always)
    func helper(_ current: Int, _ nextVal: Int) -> Int {
        return computeHash(seed: current, value: nextVal)
    }

    var state = 0
    for item in items {
        state = helper(state, item)
    }
    return state
}

@usableFromInline
internal struct FastBuffer {
    @usableFromInline
    var storage: [Int]

    @inlinable
    init(storage: [Int]) {
        self.storage = storage
    }

    @inlinable
    func fold() -> Int {
        func localSum(_ idx: Int, _ acc: Int) -> Int {
            if idx >= storage.count { return acc }
            return localSum(idx + 1, acc + storage[idx])
        }
        return localSum(0, 0)
    }
}
