// A literal where an associated type goes: 0 as a T.Acc, which its
// protocol constrains to be Numeric, compared with and assigned.
protocol N: Numeric, Comparable {
    associatedtype Acc: N
    static func widen(_ x: Self) -> Acc
}
extension Float: N { typealias Acc = Float; static func widen(_ x: Float) -> Float { x } }
extension Int32: N { typealias Acc = Int32; static func widen(_ x: Int32) -> Int32 { x } }
func relu<T: N>(_ x: T) -> T.Acc {
    var v = T.widen(x)
    if v < 0 { v = 0 }
    return v
}
print(relu(Float(-2.5)), relu(Float(3)), relu(Int32(-4)), relu(Int32(9)))
