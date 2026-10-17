// An implicit enum member inside a tuple takes its type from the
// optional-tuple result it is returned as.
enum BinaryOp { case logicalOr, logicalAnd, add }
func op(_ s: String) -> (BinaryOp, Int)? {
    switch s {
    case "||": return (.logicalOr, 1)
    case "&&": return (.logicalAnd, 2)
    case "+": return (.add, 10)
    default: return nil
    }
}
for s in ["||", "+", "?"] {
    if let (o, p) = op(s) { print(o, p) } else { print("nil") }
}
