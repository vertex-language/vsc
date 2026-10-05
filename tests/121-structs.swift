// A struct with stored properties, made by its memberwise initializer.
struct Point {
    var x: Int
    var y: Int
}
var p = Point(x: 3, y: 4)
print(p.x, p.y)
p.x = 10
print(p.x * p.y, p)
