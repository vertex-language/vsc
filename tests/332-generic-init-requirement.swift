// A generic initializer as a protocol requirement, called through a type parameter.
protocol Scaled {
    init<S: BinaryInteger>(scaling s: S)
    var raw: Int { get }
}
struct Tenths: Scaled {
    let raw: Int
    init<S: BinaryInteger>(scaling s: S) { raw = Int(s) * 10 }
}
func make<T: Scaled>(_: T.Type, _ n: UInt16) -> T { T(scaling: n) }
print(make(Tenths.self, 7).raw, Tenths(scaling: Int8(-4)).raw)
