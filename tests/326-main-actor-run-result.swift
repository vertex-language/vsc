// MainActor.run gives back what its body does, and throws what it throws.
@MainActor final class Box { static var counter = 5 }
struct Oops: Error {}
func f() async throws {
    let n = await MainActor.run { Box.counter + 1 }
    print("n", n)
    await MainActor.run { Box.counter = 9 }
    let s = await MainActor.run(resultType: String.self) { "c=\(Box.counter)" }
    print(s)
    do {
        let _: Int = try await MainActor.run { () throws -> Int in throw Oops() }
    } catch { print("threw", error is Oops) }
}
try await f()
