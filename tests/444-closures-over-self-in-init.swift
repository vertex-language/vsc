// An init, once its stored properties are set, uses a nested function that
// appends to self's property and a map whose closure calls a static.
final class Look {
    var still: [String]
    var textures: [Int]
    static func Upload(_ s: String) -> Int { s.count }
    init(_ names: [String]) {
        still = []
        textures = []
        func add(_ s: String) { self.still.append(s.uppercased()) }
        for n in names { add(n) }
        textures = names.map { Look.Upload($0) }
        let extra = { self.still.append("sky") }
        extra()
    }
}
let l = Look(["rock", "sand"])
print(l.still, l.textures)
