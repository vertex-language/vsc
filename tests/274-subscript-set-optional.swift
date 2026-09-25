// A value written through a subscript whose setter takes an optional:
// a borrowed let, a temporary, and a throwing call's result.
struct E: Error {}
struct Bag {
    var keys: [String] = []
    var values: [String: String] = [:]
    subscript(key: String) -> String? {
        get { return values[key] }
        set {
            if let v = newValue {
                if values[key] == nil { keys.append(key) }
                values[key] = v
            }
        }
    }
}

func make(_ i: Int) throws -> String {
    if i > 2 { throw E() }
    return "v\(i)"
}

func fill(_ n: Int) throws -> Bag {
    var b = Bag()
    var i = 0
    while i < n {
        let key = "k\(i)"

        b[key] = try make(i)
        let s = "s\(i)"
        b["l" + key] = s
        i += 1
    }
    return b
}

do {
    print(try fill(2).keys)
    print(try fill(5).keys)
} catch {
    print("thrown")
}
