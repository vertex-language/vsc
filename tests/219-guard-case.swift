// guard case matches a pattern and leaves the scope when it does not.
enum Token { case number(Int), word(String) }
func double(_ t: Token) -> Int? {
    guard case .number(let n) = t else {
        print("not a number")
        return nil
    }
    return n * 2
}
func firstWord(_ ts: [Token]) -> String {
    for t in ts {
        guard case let .word(w) = t, !w.isEmpty else { continue }
        return w
    }
    return "-"
}
print(double(.number(21)) as Any, double(.word("x")) as Any)
print(firstWord([.number(1), .word(""), .word("found")]))
