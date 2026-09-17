// Existentials the way a program uses them: values of every kind put in
// an Any, copied from one binding to another, kept in arrays and
// dictionaries, cast back out, and printed.
protocol Named {
    var name: String { get }
    func greet() -> String
}

struct Person: Named {
    let name: String
    let tags: [String]
    func greet() -> String { return "hi \(name)" }
}

final class Box: Named {
    let name: String
    init(name: String) { self.name = name }
    func greet() -> String { return "box \(name)" }
}

enum Pet: Named {
    case dog(String)
    case cat
    var name: String {
        switch self {
        case .dog(let n): return n
        case .cat: return "cat"
        }
    }
    func greet() -> String { return "pet \(name)" }
}

struct Wide {
    let a: Int
    let b: Int
    let c: Int
    let d: Int
    let label: String
}

func main() -> Int32 {
    var failures: Int32 = 0

    let maybe: Int? = 5
    let none: String? = nil
    print(maybe as Any, none as Any)

    let p = Person(name: "ann", tags: ["a", "b"])
    let x: Any = p
    let y = x
    var z: Any = 1
    z = y
    if let back = z as? Person {
        if back.name != "ann" || back.tags.count != 2 { failures += 1 }
    } else {
        failures += 1
    }

    let things: [Any] = [1, "two", p, maybe as Any, Wide(a: 1, b: 2, c: 3, d: 4, label: "w"), [1, 2]]
    print(things)
    var copy = things
    copy.append(Box(name: "b") as Any)
    if copy.count != 7 || things.count != 6 { failures += 1 }
    if let w = things[4] as? Wide, w.label != "w" || w.d != 4 { failures += 1 }
    if things[1] is Int || !(things[1] is String) { failures += 1 }

    let named: [Named] = [p, Box(name: "bx"), Pet.dog("rex"), Pet.cat]
    var greetings: [String] = []
    for n in named {
        greetings.append(n.greet())
    }
    let first = named[0]
    let again = first
    if again.name != "ann" { failures += 1 }
    print(greetings)

    var byKey: [String: Any] = ["n": 3, "s": "str", "p": p]
    byKey["w"] = Wide(a: 0, b: 0, c: 0, d: 0, label: "z")
    if (byKey["s"] as? String) != "str" || byKey.count != 4 { failures += 1 }

    print(failures == 0 ? "ok" : "failed \(failures)")
    return failures
}
