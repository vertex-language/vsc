// `p?.x` is the member where p holds something and nothing where it
// does not, so it is an optional whatever the member is. It used to
// answer Invalid -- the lookup ran on a doubly optional type and
// found nothing, and said nothing about it -- which made the chain
// assignable to anything at all.
struct P { var x: Int32 }

func chain(_ p: P?) -> Int32 {
    let v: Int32 = p?.x
    return v
}
