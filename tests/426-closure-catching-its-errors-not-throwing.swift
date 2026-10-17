// A closure whose every try is inside a do/catch that catches everything
// is not a throwing closure.
struct Bad: Error {}
final class Decoder {
    var left = 3
    func Next() throws -> Int {
        left -= 1
        if left == 1 { throw Bad() }
        return left
    }
}
func withLock<R>(_ body: () -> R) -> R { body() }
func drain(_ d: Decoder) -> [String] {
    withLock {
        var out: [String] = []
        while d.left > 0 {
            do {
                out.append(String(try d.Next()))
            } catch {
                out.append("caught")
            }
        }
        return out
    }
}
print(drain(Decoder()))
