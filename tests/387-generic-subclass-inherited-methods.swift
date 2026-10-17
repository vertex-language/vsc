// A generic subclass of a non-generic class keeps its base's methods in
// its class table: called from the subclass and through the base.
class Source {
    var reads = 0
    func track() { reads += 1 }
}
final class Signal<T>: Source {
    var v: T
    init(_ v: T) { self.v = v }
    var value: T { track(); return v }
}
let a = Signal(5)
_ = a.value
_ = a.value
let b = Signal("x")
print(b.value)
print(a.reads, b.reads)
let all: [Source] = [a, b]
for s in all { s.track() }
print(a.reads, b.reads)
