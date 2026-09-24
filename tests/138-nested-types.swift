// Types declared inside other types, and named from outside.
struct Card {
    enum Suit: Character { case spades = "♠", hearts = "♥" }
    struct Rank {
        let value: Int
        var name: String { value == 1 ? "A" : String(value) }
    }
    let rank: Rank
    let suit: Suit
    var name: String { rank.name + String(suit.rawValue) }
}
let c = Card(rank: Card.Rank(value: 1), suit: .spades)
print(c.name, Card(rank: .init(value: 7), suit: .hearts).name)
