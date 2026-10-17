// A for var loop variable assigned through in the body, a copy that
// leaves the array alone.
struct Particle {
    var x: Int
    var life: Int
}
let ps = [Particle(x: 0, life: 3), Particle(x: 5, life: 1)]
var moved: [Int] = []
for var p in ps {
    p.x += 1
    p.life -= 1
    if p.life > 0 { moved.append(p.x) }
}
print(moved, ps.map { $0.x })
for var s in ["a", "b"] {
    s += "!"
    print(s)
}
