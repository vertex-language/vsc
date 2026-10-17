// A non-generic overload is picked over a generic one that also fits.
final class Signal<T> {
    let v: T
    init(_ v: T) { self.v = v }
}
func Value(_ s: Signal<String>) -> String { "signal \(s.v)" }
func Value<V>(_ v: V) -> String { "value \(v)" }
print(Value(Signal("a")), Value(3), Value("b"))
func show(_ x: Int) -> String { "int \(x)" }
func show<T>(_ x: T) -> String { "generic \(x)" }
print(show(1), show(1.5))
