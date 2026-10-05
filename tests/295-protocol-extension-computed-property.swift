// A computed property a protocol extension declares, read on a conformer
// and on a generic value.
protocol Num { var n: Int { get } }
extension Num {
    var isZero: Bool { n == 0 }
    var doubled: Int { n * 2 }
}
struct S: Num { var n: Int }
print(S(n: 0).isZero, S(n: 3).isZero, S(n: 3).doubled)
func z<T: Num>(_ t: T) -> Bool { t.isZero }
print(z(S(n: 0)))
