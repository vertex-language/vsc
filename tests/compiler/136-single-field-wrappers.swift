// A struct of one field is that field. When the field is itself a
// struct of several, all of them have to be carried across -- the
// wrapper holds what it wraps, and reading through it reads what is
// inside.
struct Inner { var first: Int32; var second: Int32 }
struct Outer { var inner: Inner }
struct Twice { var outer: Outer }

func sum(_ o: Outer) -> Int32 { return o.inner.first + o.inner.second }

func main() -> Int32 {
    let o = Outer(inner: Inner(first: 20, second: 15))
    if o.inner.first != 20 { return 91 }
    if o.inner.second != 15 { return 92 }
    if sum(o) != 35 { return 93 }

    // Two wrappers deep.
    let t = Twice(outer: o)
    if t.outer.inner.first != 20 { return 94 }
    if sum(t.outer) != 35 { return 95 }

    return sum(o) + 7
}
