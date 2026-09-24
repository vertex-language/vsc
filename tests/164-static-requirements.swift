// Static and initializer requirements, and Self in a protocol.
protocol Zeroable {
    static var zero: Self { get }
    init(copying other: Self)
    static func + (a: Self, b: Self) -> Self
}
struct Money: Zeroable {
    var cents: Int
    static var zero: Money { Money(cents: 0) }
    init(cents: Int) { self.cents = cents }
    init(copying other: Money) { cents = other.cents }
    static func + (a: Money, b: Money) -> Money { Money(cents: a.cents + b.cents) }
}
func total<T: Zeroable>(_ xs: [T]) -> T { xs.reduce(T.zero, +) }
let t = total([Money(cents: 150), Money(cents: 275)])
print(t.cents, Money(copying: t).cents)
