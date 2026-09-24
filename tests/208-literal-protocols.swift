// ExpressibleBy...Literal lets a type be written as a literal.
struct Money: ExpressibleByIntegerLiteral, ExpressibleByFloatLiteral, CustomStringConvertible {
    let cents: Int
    init(integerLiteral v: Int) { cents = v * 100 }
    init(floatLiteral v: Double) { cents = Int((v * 100).rounded()) }
    var description: String { "$\(cents / 100).\(cents % 100 < 10 ? "0" : "")\(cents % 100)" }
}
struct Tags: ExpressibleByArrayLiteral, ExpressibleByStringLiteral {
    let items: [String]
    init(arrayLiteral xs: String...) { items = xs }
    init(stringLiteral s: String) { items = s.split(separator: " ").map(String.init) }
}
struct Env: ExpressibleByDictionaryLiteral {
    let pairs: [(String, Int)]
    init(dictionaryLiteral xs: (String, Int)...) { pairs = xs }
}
let price: Money = 12
let tip: Money = 1.05
let t1: Tags = ["a", "b"]
let t2: Tags = "x y z"
let env: Env = ["depth": 3, "width": 80]
print(price, tip, t1.items, t2.items, env.pairs.map { "\($0.0)=\($0.1)" })
