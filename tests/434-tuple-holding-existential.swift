// Tuples holding an existential, in a local, an array and a result.
protocol Image { func size() -> Int }
struct Big: Image {
    let a: Int, b: Int, c: Int, d: Int, e: Int
    func size() -> Int { a + b + c + d + e }
}
final class Slice: Image {
    let n: Int
    init(_ n: Int) { self.n = n }
    deinit { print("slice", n, "freed") }
    func size() -> Int { n }
}
func open() -> [(String, any Image)] {
    let s: any Image = Slice(3)
    let b: any Image = Big(a: 1, b: 2, c: 3, d: 4, e: 5)
    var disks: [(String, any Image)] = [("system", s)]
    disks.append(("data", b))
    return disks
}
do {
    let disks = open()
    for (name, img) in disks { print(name, img.size()) }
    let pair: (String, any Image) = disks[0]
    print(pair.0, pair.1.size())
}
print("done")
