// `for case P in xs` runs its body for the elements that match P, with
// what P binds, and skips the rest as `continue` would -- and a where
// clause after it skips more. Over optionals, by `let x?`, `.some` and
// `nil`, and over an enum's cases, with counted payloads let go of
// whether they matched or not.

enum Event { case click(Int, Int), key(String), idle }
func main() -> Int32 {
    var total = 0
    var names = ""
    var nils = 0
    for _ in 0..<300 {
        let xs: [Int?] = [1, nil, 3, nil, 5]
        for case let x? in xs { total += x }
        for case let x? in xs where x > 2 { total += x * 100 }
        for case nil in xs { nils += 1 }
        let ss: [String?] = ["a", nil, "bc"]
        for case .some(let s) in ss where s.count > 1 { names += s }
        for case let s? in ss { names += s }
        let events: [Event] = [.click(1, 2), .key("q"), .idle, .key("wx"), .click(3, 4)]
        for case .click(let x, let y) in events { total += x * y }
        for case .key(let k) in events where k.count == 2 { names += k }
        for case .idle in events { nils += 10 }
    }
    print(total, names.count, nils)
    return Int32(total % 256)
}
