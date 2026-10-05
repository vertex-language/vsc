// Stored properties of existential type, in a struct, a class, and assigned in an init.
protocol Shape { func area() -> Int }
struct Square: Shape { var side: Int; func area() -> Int { side * side } }
struct Holder { var s: any Shape; var tag: Any }
final class Box {
    var s: any Shape
    var value: Any
    init(_ n: Int) { s = Square(side: n); value = n * 10 }
}
var h = Holder(s: Square(side: 2), tag: "two")
print(h.s.area(), h.tag as! String)
h.s = Square(side: 5)
let b = Box(3)
print(h.s.area(), b.s.area(), b.value as! Int)
