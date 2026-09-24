// An Optional is nil or holds a value; if let takes it out.
var maybe: Int? = nil
print(maybe as Any, maybe == nil)
maybe = 5
if let v = maybe {
    print("has", v)
}
if let maybe {
    print("shorthand", maybe)
}
let parsed = Int("x")
if let p = parsed { print(p) } else { print("no value") }
