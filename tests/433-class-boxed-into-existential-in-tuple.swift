// Concrete values boxed into an existential inside each tuple of an
// array literal passed for a [(String, any P)] parameter.
protocol Image { func size() -> Int }
final class Slice: Image {
    let n: Int
    init(_ n: Int) { self.n = n }
    deinit { print("slice", n, "freed") }
    func size() -> Int { n }
}
struct Mem: Image { func size() -> Int { 64 } }
func gpt(_ parts: [(String, any Image)]) -> String {
    parts.map { "\($0.0)=\($0.1.size())" }.joined(separator: " ")
}
print(gpt([("vendor", Slice(4)), ("oem", Mem())]))
let mem = Mem()
print(gpt([("a", mem), ("b", Slice(8)), ("c", mem)]))
print("done")
