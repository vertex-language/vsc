// An enum is described by its case: alone, in an array or an optional,
// as a field, and by its description where it has one.
enum Light: CustomStringConvertible {
    case red, green
    var description: String {
        switch self {
        case .red: return "stop"
        case .green: return "go"
        }
    }
}

enum Suit {
    case hearts, spades
}

enum Shape {
    case circle(Double)
    case named(String)
    case point
    case square(Int)
}

struct Card {
    let suit: Suit
    let rank: Int
}

func main() -> Int32 {
    print(Suit.spades)
    print("\(Suit.hearts)")
    print([Suit.hearts, .spades])
    print([Light.green, .red])
    let maybe: Suit? = .hearts
    print([maybe])
    print(Card(suit: .spades, rank: 3))
    print(Shape.circle(1.5), Shape.point)
    print([Shape.named("x"), .square(4), .point, .circle(2)])
    return 0
}
