// A closure passed to a generic function returns what the call's result is
// wanted as: `.array(xs.map { .string($0) })` maps to [Value], so the
// closure's `.string` is Value's case, with no annotation.
enum Value { case string(String), number(Int), array([Value]), null }

func show(_ v: Value) -> String {
    switch v {
    case .string(let s): return "\"" + s + "\""
    case .number(let n): return String(n)
    case .array(let a): return "[" + a.map(show).joined(separator: ",") + "]"
    case .null: return "null"
    }
}

let names = ["a", "b"]
print(show(.array(names.map { .string($0) })))
let counts: [Value] = [1, 2, 3].map { .number($0 * 10) }
print(show(.array(counts)))
let empty: [String] = []
let maybe: Value = empty.isEmpty ? .null : .array(empty.map { .string($0) })
print(show(maybe))

protocol Shape { func area() -> Int }
struct Square: Shape { let s: Int; func area() -> Int { s * s } }
let shapes: [any Shape] = [1, 2].map { Square(s: $0) }
print(shapes.map { $0.area() })
