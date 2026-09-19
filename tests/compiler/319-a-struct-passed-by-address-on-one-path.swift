// A struct held in registers that one path hands to a callee by address
// -- written to a slot there -- and another path releases: the release
// on the other path must read the registers, not the slot that path
// never wrote.

enum Kind { case ident, number, dimension }

struct Token {
    var kind: Kind
    var value: String
    var unit: String
    var number: Float
    var space: Bool
}

enum Prop { case width, minWidth, color }

enum Value {
    case auto
    case length(Float, String)
    case keyword(String)
}

func lower(_ s: String) -> String { return s }

func parseLength(_ t: Token) -> Value? {
    if t.kind == .dimension { return .length(t.number, t.unit) }
    return nil
}

func parse(_ prop: Prop, _ tokens: [Token]) -> Value? {
    let t = tokens[0]
    let kw = t.kind == .ident ? lower(t.value) : ""
    switch prop {
    case .minWidth:
        if kw == "auto" { return .auto }
        return parseLength(t)
    case .color:
        return .keyword(kw)
    default:
        return nil
    }
}

func describe(_ v: Value?) -> String {
    guard let v = v else { return "nil" }
    switch v {
    case .auto: return "auto"
    case .length(let n, let u): return "\(n)\(u)"
    case .keyword(let k): return "keyword(\(k))"
    }
}

func main() -> Int32 {
    let dims = [Token(kind: .dimension, value: "5px", unit: "px", number: 5, space: false)]
    let idents = [Token(kind: .ident, value: "auto", unit: "", number: 0, space: true)]
    print(describe(parse(.minWidth, dims)))
    print(describe(parse(.width, dims)))
    print(describe(parse(.color, dims)))
    print(describe(parse(.minWidth, idents)))
    print(describe(parse(.width, idents)))
    print(describe(parse(.color, idents)))
    var i = 0
    while i < 1000 {
        _ = parse(.width, dims)
        _ = parse(.minWidth, idents)
        i += 1
    }
    print("done")
    return 0
}
