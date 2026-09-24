// switch on a tuple, with wildcards and bindings in the patterns.
func place(_ p: (Int, Int)) -> String {
    switch p {
    case (0, 0): return "origin"
    case (_, 0): return "on the x axis"
    case (0, let y): return "on the y axis at \(y)"
    case (-2...2, -2...2): return "near"
    case let (x, y): return "at \(x), \(y)"
    }
}
for p in [(0, 0), (5, 0), (0, -3), (1, 2), (9, 9)] { print(place(p)) }
