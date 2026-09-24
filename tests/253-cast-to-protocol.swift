// as?, as! and is with a protocol as the target type.
protocol Named { var name: String { get } }
struct Dog: Named { var name: String }
struct Rock {}
let things: [Any] = [Dog(name: "rex"), Rock(), 7]
for t in things {
    if let n = t as? Named { print("named", n.name) } else { print("unnamed", t is Named) }
}
let d = things[0] as! Named
print(d.name)
