// description controls how print and interpolation show a value.
struct Temperature: CustomStringConvertible {
    let celsius: Double
    var description: String { "\(celsius)°C" }
}
enum Coin: CustomStringConvertible {
    case heads, tails
    var description: String { self == .heads ? "H" : "T" }
}
struct Plain { let a = 1; let b = "two" }
let t = Temperature(celsius: 21.5)
print(t, "it is \(t)", [t], Coin.heads, [Coin.tails, .heads])
print(Plain(), String(describing: Coin.tails))
