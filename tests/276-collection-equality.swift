// == on arrays and dictionaries of Equatable structs and enums, which
// the runtime does not compare itself: through each element's ==, as
// Swift's conditional conformances do. Also derived == on an enum that
// holds arrays and dictionaries of itself.
struct F: Equatable { var p: String; var n: Int }
let a = [F(p: "x", n: 1), F(p: "y", n: 2)]
print(a == [F(p: "x", n: 1), F(p: "y", n: 2)], a == [], a != [F(p: "x", n: 1)])

let d: [String: F] = ["k": F(p: "x", n: 1)]
print(d == ["k": F(p: "x", n: 1)], d == ["k": F(p: "x", n: 2)], d != [:])
let counts: [String: Int] = ["a": 1, "b": 2]
print(counts == ["b": 2, "a": 1], counts == ["a": 1])

enum V: Equatable {
    case n(Int)
    case a([V])
    case d([String: V])
}
let x = V.a([.n(1), .d(["k": .a([])])])
print(x == x, x == V.a([.n(1)]), V.d(["k": .n(1)]) == V.d(["k": .n(2)]))
