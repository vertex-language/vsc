// A let or var declared without a value is given one later: by a throwing
// call inside do, on each branch of an if, or each time round a loop. A
// path that leaves before the assignment lets go of nothing.
struct Bad: Error {
    var code: Int
}

struct Named {
    var name: String
    var n: Int
    func weight() -> Int { return n + name.count }
}

final class Counter {
    var hits = 0
}

func make(_ n: Int) throws -> Named {
    if n < 0 {
        throw Bad(code: n)
    }
    return Named(name: "item\(n)", n: n)
}

func loaded(_ n: Int) -> Int {
    let item: Named
    do {
        item = try make(n)
    } catch let b as Bad {
        return -b.code
    } catch {
        return 100
    }
    return item.weight()
}

func pick(_ flag: Bool) -> String {
    let label: String
    if flag {
        label = "a long label that lives on the heap"
    } else {
        label = "short"
    }
    return label
}

func main() -> Int32 {
    var total = loaded(3) + loaded(-4)
    total += pick(true).count + pick(false).count
    let counter = Counter()
    for i in 0..<4 {
        var owner: Counter
        if i % 2 == 0 {
            owner = counter
        } else {
            owner = Counter()
        }
        owner.hits += i
    }
    total += counter.hits
    var words: [String]
    words = ["x", "yy"]
    words.append("zzz")
    for w in words {
        total += w.count
    }
    return Int32(total % 251)
}
