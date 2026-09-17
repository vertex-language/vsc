// A struct with no stored properties is a type with no bytes.
struct Marker { }

struct Tagged {
    var mark: Marker
    var n: Int32
}

func take(_ m: Marker, _ n: Int32) -> Int32 { return n }
func give() -> Marker { return Marker() }

extension Marker {
    func answer() -> Int32 { return 42 }
}

func main() -> Int32 {
    let m = Marker()
    if take(m, 1) != 1 { return 91 }
    if take(give(), 2) != 2 { return 92 }
    return m.answer()
}
