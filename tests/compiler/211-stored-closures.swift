// A property may hold a function, and calling the property calls it: a
// closure in a struct beside other fields, one that captures, one in a
// class field that is replaced, and one that throws.
struct Bad: Error {
    var code: Int
}

struct Handler {
    var name: String
    var run: (Int) -> Int
}

struct Single {
    var run: (Int) -> Int
}

final class Pipeline {
    var step: (Int) -> Int = { $0 }
    var check: (Int) throws -> Int = { $0 }
}

func main() -> Int32 {
    var total = 0
    let h = Handler(name: "double", run: { $0 * 2 })
    total += h.run(21)
    if h.name == "double" {
        total += 1
    }

    let base = 100
    let offset = Handler(name: "offset", run: { $0 + base })
    total += offset.run(5)

    let single = Single(run: { $0 - 3 })
    total += single.run(10)

    let p = Pipeline()
    total += p.step(7)
    p.step = { $0 * $0 }
    total += p.step(7)

    p.check = { x in
        if x < 0 {
            throw Bad(code: x)
        }
        return x + 1
    }
    do {
        total += try p.check(4)
        total += try p.check(-2)
        total += 1000
    } catch let b as Bad {
        total += -b.code * 10
    } catch {
        total += 2000
    }
    return Int32(total % 251)
}
