// A struct that holds arrays of itself, by way of other structs: a
// selector whose :not() holds selectors. Describing its fields comes
// back around to the type being described, and there is one metadata
// record and one descriptor for it, not two.

struct Pseudo {
    var name: String
    var inner: [Complex]
}

struct Part {
    var tag: String
    var pseudos: [Pseudo]
}

struct Complex {
    var parts: [Part]
}

func describe(_ c: Complex, _ depth: Int) -> String {
    var out = ""
    for p in c.parts {
        out += p.tag
        for ps in p.pseudos {
            out += ":" + ps.name + "("
            for inner in ps.inner {
                out += describe(inner, depth + 1)
            }
            out += ")"
        }
    }
    return out
}

func main() -> Int32 {
    let leaf = Complex(parts: [Part(tag: "a", pseudos: [])])
    let not = Pseudo(name: "not", inner: [leaf, Complex(parts: [Part(tag: "b", pseudos: [])])])
    let outer = Complex(parts: [Part(tag: "div", pseudos: [not]), Part(tag: "p", pseudos: [])])
    print(describe(outer, 0))
    var list: [Complex] = []
    list.append(outer)
    list.append(leaf)
    print(list.count, list[0].parts.count, list[1].parts[0].tag)
    print(outer)
    return 0
}
