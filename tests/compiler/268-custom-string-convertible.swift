// A type that is CustomStringConvertible is shown by its description:
// in string interpolation, by print, and inside an array, an optional or
// another value's reflection, where the runtime finds the conformance.
struct Span: CustomStringConvertible {
    let nanos: Int64
    var description: String { return "\(nanos)ns" }
}

enum Light: CustomStringConvertible {
    case red, green
    var description: String {
        switch self {
        case .red: return "stop"
        case .green: return "go"
        }
    }
}

struct Plain {
    let n: Int
}

struct Holder {
    let span: Span
    let plain: Plain
}

// A stored property meets the requirement as well as a computed one.
struct Named: CustomStringConvertible {
    let description: String
}

func main() -> Int32 {
    var failures: Int32 = 0
    let s = Span(nanos: 5)
    if "\(s)" != "5ns" { failures += 1 }
    if "took \(s) at \(Light.red)" != "took 5ns at stop" { failures += 1 }
    print(s)
    print(Light.green, s)
    let maybe: Span? = s
    if "\(maybe!)" != "5ns" { failures += 1 }
    print([s, Span(nanos: 7)])
    print([maybe])
    print(Holder(span: s, plain: Plain(n: 4)))
    print(Plain(n: 3))
    print([Named(description: "stored")])
    print(failures == 0 ? "ok" : "failed \(failures)")
    return failures
}
