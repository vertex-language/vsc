// Two generic functions of one name whose parameters differ only in the
// protocol their type parameter is constrained to: each call is the one
// whose constraint the argument meets.

protocol Reader {
    mutating func read() throws -> Int
}

protocol AsyncReader {
    mutating func read() async throws -> Int
}

struct Mem: Reader {
    var left: Int
    mutating func read() throws -> Int {
        if left == 0 { return 0 }
        left -= 1
        return 1
    }
}

struct Net: AsyncReader {
    var left: Int
    mutating func read() async throws -> Int {
        if left == 0 { return 0 }
        left -= 1
        return 2
    }
}

func total<R: Reader>(_ r: inout R) throws -> Int {
    var t = 0
    while true {
        let n = try r.read()
        if n == 0 { return t }
        t += n
    }
}

func total<R: AsyncReader>(_ r: inout R) async throws -> Int {
    var t = 0
    while true {
        let n = try await r.read()
        if n == 0 { return t }
        t += n
    }
}

func main() async -> Int32 {
    var m = Mem(left: 3)
    var n = Net(left: 3)
    do {
        let a = try total(&m)
        let b = try await total(&n)
        return Int32(a * 10 + b)
    } catch {
        return 1
    }
}
