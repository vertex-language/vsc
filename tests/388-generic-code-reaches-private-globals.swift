// Specialized generic code reaches a private global and makes an instance
// of a private generic class that only it names.
private var created = 0
private final class Derived<T> {
    let v: T
    init(_ v: T) { self.v = v; created += 1 }
}
func make<T>(_ v: T) -> String {
    let d = Derived(v)
    return "\(d.v)"
}
print(make(1), make("two"), make([3]))
print(created)
