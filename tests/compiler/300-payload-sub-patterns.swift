// A case whose payload pattern matches rather than binds -- `.code(0)`,
// `.pair(_, true)`, `.text("hi", let k)` -- covers only the payloads that
// match, and the cases after it are tried for the rest, as after a where
// clause that does not hold. What a case bound and did not match is let go.

enum Msg {
    case code(Int)
    case pair(Int, Bool)
    case text(String, Int)
    case quit
}

func describe(_ m: Msg) -> String {
    switch m {
    case .code(0): return "zero"
    case .code(1), .quit: return "one-or-quit"
    case .code(let n) where n < 0: return "neg \(n)"
    case .code(let n): return "code \(n)"
    case .pair(_, true): return "pair-true"
    case .pair(3, false): return "three-false"
    case let .pair(n, false): return "pair \(n)"
    case .text("hi", let k): return "hi x\(k)"
    case .text(let s, 0): return "empty-count " + s
    case .text(let s, let k): return s + String(k)
    }
}

func main() -> Int32 {
    let ms: [Msg] = [.code(0), .code(1), .code(-4), .code(9), .quit, .pair(3, true), .pair(3, false),
                     .pair(8, false), .text("hi", 2), .text("yo", 0), .text("ab", 5)]
    var out: [String] = []
    for m in ms { out.append(describe(m)) }
    print(out)
    return Int32(out.count)
}
