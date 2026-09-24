// fallthrough continues into the next case's body without testing it.
func describe(_ n: Int) -> String {
    var s = "\(n) is"
    switch n {
    case 2, 3, 5, 7:
        s += " prime and"
        fallthrough
    default:
        s += " an integer"
    }
    return s
}
print(describe(5))
print(describe(8))
