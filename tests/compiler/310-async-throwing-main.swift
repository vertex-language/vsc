// A main that is both async and throws: awaited and tried from the top,
// an error it catches handled, its status the program's.

enum Err: Error { case bad }

func fetch(_ n: Int) async throws -> Int {
    if n < 0 { throw Err.bad }
    return n * 2
}

func main() async throws -> Int32 {
    let a = try await fetch(3)
    print(a)
    do {
        _ = try await fetch(-1)
    } catch {
        print("caught \(error)")
    }
    return Int32(try await fetch(4))
}
