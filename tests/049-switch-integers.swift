// switch on an integer: single values, lists, and default.
func name(_ n: Int) -> String {
    switch n {
    case 0:
        return "zero"
    case 1, 3, 5, 7, 9:
        return "odd digit"
    case 2, 4, 6, 8:
        return "even digit"
    default:
        return "big"
    }
}
for n in [0, 3, 8, 42] { print(n, name(n)) }
