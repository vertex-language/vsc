// An optional whose payload is more than one register. Its image is
// the payload beside a tag byte, and switching over one passed the
// some arm the payload's *first* register -- so everything about
// optionals was limited to payloads of one, and a struct of two
// fields could not be chained through, bound, unwrapped or defaulted.
//
// A block argument may be several registers: the arm declares one
// parameter per leaf, the way a struct parameter arrives as its
// words. lower already did that on the declaring side; what it did
// not do was pass the whole payload on the edge.
struct Point {
    var x: Int32
    var y: Int32
}

func chained(_ p: Point?) -> Int32 {
    return p?.x ?? -1
}

func bound(_ p: Point?) -> Int32 {
    if let q = p { return q.x + q.y }
    return -2
}

func forced(_ p: Point?) -> Int32 {
    return (p!).y
}

func defaulted(_ p: Point?, _ fallback: Point) -> Int32 {
    let r = p ?? fallback
    return r.x + r.y
}

// Three fields, so the payload is more registers still.
struct Wide {
    var a: Int32
    var b: Int32
    var c: Int32
}

func third(_ w: Wide?) -> Int32 {
    if let v = w { return v.c }
    return 0
}

func main() -> Int32 {
    let p: Point? = Point(x: 5, y: 6)
    let w: Wide? = Wide(a: 1, b: 2, c: 7)
    return chained(p) + chained(nil) + bound(p) + bound(nil)
         + forced(p) + defaulted(nil, Point(x: 3, y: 4)) + third(w)
}
