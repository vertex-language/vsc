// if let binding an optional tuple that holds a class, with a further
// condition on one of its elements.
final class Thing {
    let name: String
    init(_ name: String) { self.name = name }
    deinit { print(name, "freed") }
}
func pick(_ things: [Thing]) -> (Thing, Float)? {
    things.isEmpty ? nil : (things[0], Float(things.count))
}
var got: Thing? = nil
do {
    let things = [Thing("cow")]
    if let (t, d) = pick(things), d < 2 { got = t }
    if let (t, d) = pick([Thing("pig"), Thing("hen")]), d < 2 { got = t }
}
print(got?.name ?? "none")
got = nil
print("done")
