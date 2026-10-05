// A result builder turns a block of statements into one value.
@resultBuilder
struct Lines {
    static func buildBlock(_ parts: [String]...) -> [String] { parts.flatMap { $0 } }
    static func buildExpression(_ s: String) -> [String] { [s] }
    static func buildOptional(_ p: [String]?) -> [String] { p ?? [] }
    static func buildEither(first p: [String]) -> [String] { p }
    static func buildEither(second p: [String]) -> [String] { p }
    static func buildArray(_ ps: [[String]]) -> [String] { ps.flatMap { $0 } }
}
func document(verbose: Bool, @Lines _ body: (Bool) -> [String]) -> String {
    body(verbose).joined(separator: "\n")
}
print(document(verbose: true) { v in
    "title"
    if v { "details" }
    if v { "long" } else { "short" }
    for i in 1...2 { "item \(i)" }
})
