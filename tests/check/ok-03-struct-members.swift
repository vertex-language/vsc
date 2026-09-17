struct Point {
    var x: Double
    var y = 0.0
    func magnitudeSquared() -> Double {
        return x * x + y * y
    }
}
func use(_ p: Point) -> Double {
    return p.x + p.magnitudeSquared()
}
