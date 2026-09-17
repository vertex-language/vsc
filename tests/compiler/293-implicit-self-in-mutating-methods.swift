// Inside a mutating method, a method named without `self.` that does not
// mutate is given self's value, and one that does is given self's storage.
// A static property's initializer names static methods written after it.

final class Log {
    var lines: [String] = []
}

struct Cipher {
    var key: [Int]
    var counter = 0
    let log: Log
    static let rounds = defaultRounds()

    func nonce() -> [Int] { var n = key; n.append(counter); return n }
    func describe() -> String { return "c" + String(counter) }
    mutating func bump() { counter += 1 }
    static func defaultRounds() -> Int { return 20 }

    mutating func next() -> Int {
        let n = nonce()
        bump()
        log.lines.append(describe())
        key = n
        return n.count + Cipher.rounds
    }
}

func main() -> Int32 {
    var c = Cipher(key: [1, 2], log: Log())
    var total = 0
    for _ in 0..<50 {
        total += c.next()
    }
    print(total, c.counter, c.key.count, c.log.lines.count, c.log.lines[49])
    return Int32(total % 256)
}
