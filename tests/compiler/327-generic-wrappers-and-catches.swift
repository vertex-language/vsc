// A generic function called inside a do/catch, and a generic struct
// that calls a protocol requirement on the value it wraps: the call is
// the wrapped type's method, not the wrapper's own of the same name.

enum Bad: Error { case broken(Int) }

protocol Source {
    mutating func next() throws -> Int
}

struct Counter: Source {
    var n: Int
    mutating func next() throws -> Int {
        n += 1
        if n > 3 { throw Bad.broken(n) }
        return n
    }
}

struct Doubled<S: Source>: Source {
    var inner: S
    mutating func next() throws -> Int {
        return try inner.next() * 2
    }
}

func sum<S: Source>(_ s: inout S) throws -> Int {
    var total = 0
    while true {
        total += try s.next()
    }
}

func main() -> Int32 {
    var status: Int32 = 0

    var d = Doubled(inner: Counter(n: 0))
    do {
        let a = try d.next()
        let b = try d.next()
        if a == 2 && b == 4 { status += 1 }
    } catch {
        status += 100
    }

    var c = Counter(n: 0)
    do {
        _ = try sum(&c)
        status += 100
    } catch Bad.broken(let n) {
        if n == 4 { status += 2 }
    } catch {
        status += 100
    }

    var w = Doubled(inner: Counter(n: 0))
    let total = try? sum(&w)
    if total == nil && w.inner.n == 4 { status += 4 }
    return status
}
