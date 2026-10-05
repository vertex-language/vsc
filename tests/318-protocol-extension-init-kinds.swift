// Initializers a protocol's extension declares, further: a failable one,
// one handing on to a conformer's own init(n:), and one made for an enum.
protocol Made { init(n: Int); var n: Int { get } }
extension Made {
    init(twice m: Int) { self.init(n: m * 2) }
    init?(text: String) { guard let v = Int(text) else { return nil }; self.init(n: v) }
}
struct S: Made { var n: Int; init(n: Int) { self.n = n + 1 } }
enum E: Made { case a, b
    init(n: Int) { self = n > 0 ? .b : .a }
    var n: Int { self == .a ? 0 : 1 } }
print(S(twice: 4).n, S(text: "7")?.n ?? -1, S(text: "x")?.n ?? -1, E(twice: 3).n, E(twice: 0).n)
