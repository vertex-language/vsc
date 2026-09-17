// A closure uses what is bound around it: an adder returned from the
// function whose parameter it keeps, integers and strings captured by
// value, closures passed through ordinary, generic and throwing calls,
// and a closure that captures a closure that captures.
struct Bad: Error {
    var code: Int
}

func makeAdder(_ n: Int) -> (Int) -> Int {
    return { $0 + n }
}

func apply(_ f: (Int) -> Int, _ x: Int) -> Int {
    return f(x)
}

func twice<T>(_ f: (T) -> T, _ x: T) -> T {
    return f(f(x))
}

func attempt(_ f: (Int) throws -> Int, _ x: Int) -> Int {
    do {
        return try f(x)
    } catch let b as Bad {
        return b.code
    } catch {
        return -1
    }
}

func main() -> Int32 {
    var total = 0
    let add5 = makeAdder(5)
    let add10 = makeAdder(10)
    total += add5(1) + add10(1) + add5(add10(0))

    let base = 7
    total += apply({ $0 * base }, 3)
    total += twice({ $0 + base }, 1)

    let name = "vertex"
    let greet = { (s: String) -> String in
        return name + " " + s
    }
    if greet("captures") == "vertex captures" {
        total += 100
    }

    let limit = 50
    total += attempt({ x in
        if x > limit {
            throw Bad(code: limit)
        }
        return x + limit
    }, 5)
    total += attempt({ x in
        if x > limit {
            throw Bad(code: limit)
        }
        return x
    }, 99)

    let scale = 3
    let scaled = { (x: Int) -> Int in
        return add5(x) * scale
    }
    total += scaled(1)

    return Int32(total % 251)
}
