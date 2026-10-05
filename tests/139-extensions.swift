// Extensions add methods, computed properties and initializers to a type, standard ones too.
extension Int {
    var squared: Int { self * self }
    func times(_ f: () -> Void) { for _ in 0..<self { f() } }
    mutating func negate() { self = -self }
}
extension String {
    init(twice s: String) { self = s + s }
}
struct Vec { var x = 0, y = 0 }
extension Vec {
    init(both n: Int) { self.init(x: n, y: n) }
    var sum: Int { x + y }
}
var n = 7
n.negate()
3.times { print("tick") }
print(5.squared, n, String(twice: "ab"), Vec(both: 4).sum, Vec().sum)
