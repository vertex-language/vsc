// A payload enum is stored as whole words, so it takes whole words:
// what follows it in a class or a struct is not overwritten when it is
// assigned, and an optional of a two-word enum keeps its tag past them.

enum Length: Equatable {
    case auto
    case px(Float)
    case calc(Float, Float)
}

struct Radii: Equatable {
    var a: Float
    var b: Float
}

final class Style {
    var display: Int32 = 0
    var width: Length = .auto
    var top: Float = 0
    var radii: Radii = Radii(a: 0, b: 0)
    var flag: Bool = false
    var margin: Length = .px(0)
    var pad: Float = 0
    var name: String = ""
}

struct Packed {
    var flag: Bool
    var length: Length
    var tail: UInt8
    var text: String
}

func describe(_ l: Length?) -> String {
    guard let l = l else { return "nil" }
    switch l {
    case .auto: return "auto"
    case .px(let v): return "\(v)px"
    case .calc(let a, let b): return "calc(\(a) + \(b)%)"
    }
}

func main() -> Int32 {
    let s = Style()
    s.top = 1
    s.pad = 4
    s.name = "kept"
    s.width = .calc(5, 6)
    s.radii.a = 7
    s.flag = true
    s.margin = .px(9)
    print(s.top, s.pad, s.name, s.width == .calc(5, 6), s.radii.a, s.flag, s.margin == .px(9))
    var p = Packed(flag: true, length: .auto, tail: 200, text: "t")
    p.length = .calc(1, 2)
    print(p.flag, p.tail, p.text, p.length == .calc(1, 2))
    var list: [Packed] = [p]
    list.append(Packed(flag: false, length: .px(3), tail: 7, text: "u"))
    for q in list { print(q.flag, q.tail, q.text, describe(q.length)) }
    let maybe: [Length?] = [nil, .calc(2, 3), .px(4)]
    for m in maybe { print(describe(m)) }
    return 0
}
