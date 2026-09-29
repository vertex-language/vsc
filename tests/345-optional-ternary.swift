// A ternary whose one arm is nil makes an optional that later code can use.
func name(_ i: Int) -> String { "n\(i)" }
func pick(_ cond: Bool) -> String {
    let x = cond ? name(1) : nil
    if let x { return x.uppercased() }
    return x ?? "none"
}
let y: Int? = pick(true).count > 1 ? 5 : nil
print(pick(true), pick(false), y.map { $0 + 1 } ?? 0)
