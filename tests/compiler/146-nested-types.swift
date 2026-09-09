// A type declared inside another type.
//
// Nesting decides one thing and nothing else: the name. `Chart.Point`
// is a struct like any other -- same layout, same registers, same
// methods -- and what being inside Chart changes is that its symbol
// says Chart before it says Point, and that it is reached through the
// outer name rather than on its own.
//
// This is how Swift's own libraries are shaped, which is why it
// matters: String.Index, Data.Deallocator, Font.Weight. An interface
// that names one and a compiler that cannot read it is a compiler
// that cannot read the library.
struct Chart {
    var scale: Int32

    struct Point {
        var x: Int32
        var y: Int32
        func sum() -> Int32 { return x + y }
    }

    enum Axis { case horizontal, vertical }

    // The inner name is in scope inside the outer type without
    // saying the outer one again.
    func origin() -> Point { return Point(x: 0, y: scale) }
}

// From outside, both names.
func plot(_ p: Chart.Point) -> Int32 { return p.sum() }
func along(_ a: Chart.Axis) -> Int32 {
    switch a {
    case .horizontal: return 1
    case .vertical: return 2
    }
}
func scaled(_ p: Chart.Point, by n: Int32) -> Chart.Point {
    return Chart.Point(x: p.x * n, y: p.y * n)
}

// Two levels of it.
struct Outer {
    struct Middle {
        struct Inner { var v: Int32 }
    }
}
func deep(_ i: Outer.Middle.Inner) -> Int32 { return i.v }

func main() -> Int32 {
    let p = Chart.Point(x: 40, y: 2)
    if p.sum() != 42 { return 91 }
    if plot(p) != 42 { return 92 }
    if scaled(p, by: 2).sum() != 84 { return 93 }
    if along(Chart.Axis.horizontal) != 1 { return 94 }
    if along(.vertical) != 2 { return 95 }
    if Chart(scale: 7).origin().sum() != 7 { return 96 }
    if deep(Outer.Middle.Inner(v: 5)) != 5 { return 97 }
    return plot(Chart.Point(x: 20, y: 22))
}
