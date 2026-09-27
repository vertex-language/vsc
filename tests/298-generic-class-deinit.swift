// A generic class's deinit runs, and releases what it held.
final class Tag { let s: String; init(_ s: String) { self.s = s }; deinit { print("tag", s) } }
final class Holder<T> {
    let v: T
    init(_ v: T) { self.v = v }
    deinit { print("holder") }
}
do { _ = Holder(1) }
do { _ = Holder(Tag("a")) }
print("done")
