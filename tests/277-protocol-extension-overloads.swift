// A protocol's extension adds an overload of a method its conformers
// declare: a call that fits only the extension's finds it. And an array
// literal of leading-dot members compared with an array takes the
// array's element type.
protocol Tok: AnyObject {
    func Decode(_ ids: [Int], removeSpecial: Bool, special: Bool) -> String
}
extension Tok {
    func Decode(_ ids: [Int]) -> String { return Decode(ids, removeSpecial: false, special: false) }
}
final class S: Tok {
    func Decode(_ ids: [Int], removeSpecial: Bool, special: Bool) -> String { return "\(ids.count) \(removeSpecial) \(special)" }
}
let s = S()
print(s.Decode([3]), s.Decode([1, 2], removeSpecial: true, special: true))

enum Kind { case normal, control, byte }
let kinds: [Kind] = [.normal, .byte]
print(kinds == [.normal, .byte], kinds != [.control])
