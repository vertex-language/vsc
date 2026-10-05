// A type nested in a generic type takes the outer type's arguments.
struct Box<T> {
    struct Inner { var v: T }
    var inner: Inner
}
let i = Box<Int>.Inner(v: 3)
let b = Box(inner: Box<String>.Inner(v: "s"))
print(i.v, b.inner.v)
