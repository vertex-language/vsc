// switch cases can be ranges, open on either end.
func size(_ n: Int) -> String {
    switch n {
    case ..<0: return "negative"
    case 0: return "zero"
    case 1..<10: return "small"
    case 10...99: return "medium"
    case 100...: return "large"
    default: return "unreachable"
    }
}
for n in [-4, 0, 7, 10, 99, 1000] { print(n, size(n)) }
