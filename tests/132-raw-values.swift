// Raw values: implicit and explicit, and init(rawValue:) that can fail.
enum Planet: Int {
    case mercury = 1, venus, earth, mars
}
enum Suit: String {
    case hearts, spades = "S"
}
print(Planet.earth.rawValue, Planet(rawValue: 4) as Any, Planet(rawValue: 9) as Any)
print(Suit.hearts.rawValue, Suit.spades.rawValue, Suit(rawValue: "S") as Any)
