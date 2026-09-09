// A struct inside a struct is laid out inline.
struct Inner {
    var a: Int32
    var b: Int32
}

struct Outer {
    var head: Int32
    var inner: Inner
}

func main() -> Int32 {
    let o = Outer(head: 1, inner: Inner(a: 2, b: 3))
    return o.head * 100 + o.inner.a * 10 + o.inner.b
}
