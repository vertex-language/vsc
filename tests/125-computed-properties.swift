// Computed properties, read-only and with a setter.
struct Rect {
    var width: Double
    var height: Double
    var area: Double { width * height }
    var side: Double {
        get { (width + height) / 2 }
        set {
            width = newValue
            height = newValue
        }
    }
}
var r = Rect(width: 2, height: 8)
print(r.area, r.side)
r.side = 3
print(r.width, r.height, r.area)
