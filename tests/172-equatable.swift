// Equatable is synthesized for structs and enums, or written by hand.
struct Point: Equatable { var x, y: Int }
enum Status: Equatable { case ok, failed(code: Int) }
struct Loose: Equatable {
    var name: String
    static func == (a: Loose, b: Loose) -> Bool { a.name.lowercased() == b.name.lowercased() }
}
print(Point(x: 1, y: 2) == Point(x: 1, y: 2), Point(x: 1, y: 2) != Point(x: 2, y: 1))
print(Status.failed(code: 3) == .failed(code: 3), Status.ok == .failed(code: 0))
print(Loose(name: "ABC") == Loose(name: "abc"), [Point(x: 0, y: 0)].contains(Point(x: 0, y: 0)))
