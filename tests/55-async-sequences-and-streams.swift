// §6.6 / §5.2 AsyncSequence, AsyncIteratorProtocol, and for-await Loops

struct Countdown: AsyncSequence {
    typealias Element = Int

    let start: Int

    struct AsyncIterator: AsyncIteratorProtocol {
        var current: Int

        mutating func next() async -> Int? {
            guard current > 0 else { return nil }
            let val = current
            current -= 1
            return val
        }
    }

    func makeAsyncIterator() -> AsyncIterator {
        return AsyncIterator(current: start)
    }
}

enum StreamError: Error {
    case interrupted
}

struct ThrowingCounter: AsyncSequence {
    typealias Element = String

    struct AsyncIterator: AsyncIteratorProtocol {
        var count = 0
        mutating func next() async throws -> String? {
            if count > 3 {
                throw StreamError.interrupted
            }
            count += 1
            return "Item \(count)"
        }
    }

    func makeAsyncIterator() -> AsyncIterator {
        return AsyncIterator()
    }
}

func testAsyncLoops() async {
    for await number in Countdown(start: 5) {
        print("Count: \(number)")
    }

    do {
        for try await msg in ThrowingCounter() {
            print("Received: \(msg)")
        }
    } catch {
        print("Error: \(error)")
    }
}
