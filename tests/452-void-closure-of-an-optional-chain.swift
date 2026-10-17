// A one-expression closure whose expression is an optional chain (a
// Void?) where a function returning Void is wanted: the value is dropped.
final class Counter {
    var n = 0
    func bump() { n += 1 }
}
final class Holder {
    var callback: (() -> Void)? = nil
}
let counter = Counter()
let holder = Holder()
holder.callback = { [weak counter] in counter?.bump() }
holder.callback?()
var f: () -> Void = { [weak counter] in counter?.bump() }
f()
let fs: [() -> Void] = [{ counter.bump() }, { [weak counter] in counter?.bump() }]
for g in fs { g() }
print(counter.n)
