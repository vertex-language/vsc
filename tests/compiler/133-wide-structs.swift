// Past four words a struct does not fit in the registers a call has,
// so Swift passes and returns one by address: a parameter is a
// pointer to it, and a result is written into storage the caller set
// aside. That is sret, and it is why such a value never lives in
// registers at all -- there are not enough.
struct Wide {
    var a: Int
    var b: Int
    var c: Int
    var d: Int
    var e: Int
    var f: Int
}

func make() -> Wide { return Wide(a: 1, b: 2, c: 3, d: 4, e: 5, f: 6) }
func sum(_ w: Wide) -> Int { return w.a + w.b + w.c + w.d + w.e + w.f }
func last(_ w: Wide) -> Int { return w.f }

// Wide in, wide out.
func doubled(_ w: Wide) -> Wide {
    return Wide(a: w.a * 2, b: w.b * 2, c: w.c * 2, d: w.d * 2, e: w.e * 2, f: w.f * 2)
}

func main() -> Int32 {
    let w = make()
    if sum(w) != 21 { return 91 }
    if last(w) != 6 { return 92 }
    if w.a != 1 { return 93 }
    if w.f != 6 { return 94 }

    let d = doubled(w)
    if sum(d) != 42 { return 95 }
    if d.a != 2 { return 96 }

    // Through two calls, so the storage each set aside is its own.
    if sum(doubled(doubled(w))) != 84 { return 97 }

    return Int32(sum(d))
}
