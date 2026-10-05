// A payload that is itself an enum is covered case by case: .c(.x) leaves
// .c(.y) out.
enum Inner { case x, y }
enum Outer { case b, c(Inner) }

func pick(_ o: Outer) -> Int {
    switch o {
    case .b: return 1
    case .c(.x): return 2
    }
}
