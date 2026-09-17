// Compiled by this compiler, against swiftc's own interface.
//
// Every symbol here has two names in it -- the outer type and the
// inner one -- and mangling only the inner one produces a symbol the
// linker cannot find, with a demangled name that reads correctly.
import Charts2

func main() -> Int32 {
    let p = Chart.Point(x: 40, y: 2)
    if p.x != 40 { return 91 }
    if p.sum() != 42 { return 92 }
    if plot(p) != 42 { return 93 }

    let big = scaled(p, by: 2)
    if big.sum() != 84 { return 94 }

    if along(Chart.Axis.horizontal) != 1 { return 95 }
    if along(.vertical) != 2 { return 96 }

    // The outer type itself still works as a type.
    let c = Chart(scale: 3)
    if c.scale != 3 { return 97 }

    return plot(Chart.Point(x: 20, y: 22))
}
