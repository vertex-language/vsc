// A struct whose payload enum follows a smaller field: the enum is
// lowered as whole words, so it starts on one, and the metadata that
// copies the struct into an array has to say the same.

enum Prop {
    case display
    case width
    case color
}

enum Length: Equatable {
    case auto
    case px(Float)
    case percent(Float)
}

struct Decl {
    var prop: Prop
    var length: Length
}

struct Padded {
    var flag: Bool
    var length: Length
    var tail: UInt8
}

func describe(_ l: Length) -> String {
    switch l {
    case .auto: return "auto"
    case .px(let v): return "\(v)px"
    case .percent(let p): return "\(p)%"
    }
}

func main() -> Int32 {
    var decls: [Decl] = []
    decls.append(Decl(prop: .width, length: .px(50)))
    decls.append(Decl(prop: .display, length: .auto))
    decls.append(Decl(prop: .color, length: .percent(12.5)))
    for d in decls {
        print(d.prop, describe(d.length))
    }
    decls[1].length = .px(7)
    print(describe(decls[1].length), decls.count)

    var padded: [Padded] = [Padded(flag: true, length: .percent(3), tail: 9)]
    padded.append(Padded(flag: false, length: .px(1.5), tail: 200))
    for p in padded {
        print(p.flag, describe(p.length), p.tail)
    }
    let copy = padded[1]
    print(copy.length == .px(1.5), padded[0].length == .auto)
    return 0
}
