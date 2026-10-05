// if let of a ?? b, where both sides are optional.
func lookup(_ k: String) -> Int? { k == "a" ? 1 : nil }
func first(_ a: String, _ b: String) -> String {
    if let g = lookup(a) ?? lookup(b) { return "found \(g)" }
    return "neither"
}
print(first("a", "z"), first("z", "a"), first("y", "z"))
