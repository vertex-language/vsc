// A switch over a tuple of enums whose payloads hold arrays, with two
// patterns in one case and payloads released on every path.
struct Field { let heights: [Float] }
enum Shape {
    case plane(Float)
    case sphere(Float)
    case heightfield(Field)
}
func collide(_ a: Shape, _ b: Shape) -> String {
    switch (a, b) {
    case (.plane, .heightfield(_)), (.heightfield(_), .plane):
        return "plane-field"
    case (.sphere(let r), .heightfield(let f)), (.heightfield(let f), .sphere(let r)):
        return "sphere-field \(r) \(f.heights.count)"
    case (.sphere(let r1), .sphere(let r2)):
        return "spheres \(r1 + r2)"
    default:
        return "other"
    }
}
let f = Shape.heightfield(Field(heights: [0, 1, 2]))
print(collide(.plane(0), f), collide(f, .plane(1)))
print(collide(.sphere(2), f), collide(f, .sphere(3)))
print(collide(.sphere(1), .sphere(2)), collide(f, f))
