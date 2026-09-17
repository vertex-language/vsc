// Initializers that take as many parameters as each other are told apart
// by their labels: each has a body of its own.
struct Span {
    let nanos: Int64
    let label: String
    init(nanos: Int64) { self.nanos = nanos; self.label = "" }
    init(label: String) { self.nanos = 7; self.label = label }
    init(seconds: Int64) { self.nanos = seconds * 1_000_000_000; self.label = "s" }
}

func main() -> Int32 {
    var failures: Int32 = 0
    let a = Span(nanos: 3)
    let b = Span(label: "x")
    let c = Span(seconds: 2)
    if a.nanos != 3 || a.label != "" { failures += 1 }
    if b.nanos != 7 || b.label != "x" { failures += 1 }
    if c.nanos != 2_000_000_000 || c.label != "s" { failures += 1 }
    print(a.nanos, b.label, c.nanos)
    return failures
}
