// A generic property wrapper on a class's properties, read and written
// through an instance.
@propertyWrapper struct Box<T> {
    var wrappedValue: T
}
final class H {
    @Box var next = "x"
    @Box var count = 0
}
let h = H()
print(h.next, h.count)
h.next = "y"
h.count += 2
print(h.next, h.count)
