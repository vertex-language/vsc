// A generic class is made with its declared initializer for each type it
// is instantiated with, its type argument inferred from what is passed.
// Its properties with defaults have them, its methods change and read the
// shared instance, and what it holds is let go when the last reference is.
var freed = 0

final class Tracker {
    let id: Int
    init(id: Int) {
        self.id = id
    }
    deinit {
        freed += 1
    }
}

final class Box<T> {
    var value: T
    var hits: Int = 0

    init(value: T) {
        self.value = value
    }

    func get() -> T {
        hits += 1
        return value
    }

    func set(_ v: T) {
        value = v
    }
}

func run() -> Int {
    let a = Box(value: 12)
    let alias = a
    alias.set(20)
    let b = Box(value: "a string long enough to live on the heap")
    let c = Box(value: Tracker(id: 3))
    var total = a.get() + b.get().count + c.get().id
    total += a.hits + b.hits + c.hits
    return total
}

func main() -> Int32 {
    let n = run()
    return Int32(n + freed * 100)
}
