// guard let binding an optional tuple's elements, which own references.
func split(_ s: String) -> (String, String)? {
    guard let i = s.firstIndex(of: "=") else { return nil }
    return (String(s[..<i]), String(s[s.index(after: i)...]))
}
func show(_ s: String) -> String {
    guard let (k, v) = split(s) else { return "none" }
    return k + " is " + v
}
print(show("a=1"), show("b"), show("name=vsc"))
