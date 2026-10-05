// A local function declared inside a switch case, called by later
// statements of the case.
func describe(_ n: Int) -> String {
    switch n {
    case 0:
        return "zero"
    default:
        func sign(_ v: Int) -> String { v < 0 ? "negative" : "positive" }
        let s = sign(n)
        return "\(s) \(abs(n))"
    }
}
print(describe(0), describe(-3), describe(8))
