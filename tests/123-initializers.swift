// Initializers of a struct's own, delegating to one another.
struct Celsius {
    var degrees: Double
    init(degrees: Double) { self.degrees = degrees }
    init(fahrenheit f: Double) { self.init(degrees: (f - 32) * 5 / 9) }
    init() { self.init(degrees: 0) }
}
print(Celsius(degrees: 21.5).degrees, Celsius(fahrenheit: 212).degrees, Celsius().degrees)
