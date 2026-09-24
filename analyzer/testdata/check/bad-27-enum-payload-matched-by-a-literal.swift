// A case whose payload is matched by a number covers only that number:
// .a(1, _) leaves every other .a out.
enum E { case a(Int, Bool), b }

func pick(_ e: E) -> Int {
    switch e {
    case .a(1, _): return 1
    case .b: return 2
    }
}
