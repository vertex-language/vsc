// Static properties and methods belong to the type.
struct Units {
    static let perKilo = 1000
    static var made = 0
    static func grams(_ kg: Int) -> Int {
        made += 1
        return kg * perKilo
    }
}
print(Units.grams(3), Units.grams(5), Units.made, Units.perKilo)
