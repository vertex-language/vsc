// A class with no init whose stored collections default to empty
// literals: a dictionary keyed by a struct, an array of optional structs.
struct Key: Hashable {
    let a: Int
    let b: Int
}
struct Binding { let slot: Int }
final class Units {
    var d: [Key: String] = [:]
    var bindings: [Binding?] = []
}
let u = Units()
u.d[Key(a: 1, b: 2)] = "x"
u.bindings.append(nil)
u.bindings.append(Binding(slot: 3))
print(u.d[Key(a: 1, b: 2)]!, u.bindings.count, u.bindings[1]!.slot, u.bindings[0] == nil)
