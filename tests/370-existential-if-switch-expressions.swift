// An if or switch expression whose value is an existential: each branch
// is a different conforming type, written into the one existential.
protocol Shape { func describe() -> String }
struct Square: Shape { let side: Int; func describe() -> String { "square \(side)" } }
final class Circle: Shape {
    let r: Int
    init(r: Int) { self.r = r }
    func describe() -> String { "circle \(r)" }
}
struct Named: Shape { let name: String; func describe() -> String { name } }

func pick(_ n: Int) -> any Shape {
    if n == 0 { Square(side: 2) } else if n == 1 { Circle(r: 3) } else { Named(name: "other \(n)") }
}

func kind(_ n: Int) -> any Shape {
    switch n {
    case 0: Square(side: 5)
    case 1: Circle(r: 7)
    default: Named(name: "many")
    }
}

for i in 0..<3 {
    print(pick(i).describe(), kind(i).describe())
}
let flag = [1, 2].count == 2
let s: any Shape = if flag { Circle(r: 1) } else { Square(side: 1) }
print(s.describe())
let make: (Int) -> any Shape = { n in
    switch n { case 0: Named(name: "zero") default: Circle(r: n) }
}
print(make(0).describe(), make(4).describe())
var shapes: [any Shape] = []
for i in 0..<3 {
    let one: any Shape = if i % 2 == 0 { Square(side: i) } else { Named(name: "odd") }
    shapes.append(one)
}
print(shapes.map { $0.describe() })
