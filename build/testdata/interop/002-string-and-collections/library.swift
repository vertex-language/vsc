// String and the standard library's collections, behind an integer
// surface.
//
// Every one of these is generic, reference counted, or both. This
// compiler has none of that and does not need it here: what it needs
// is for a call to land in the right function with the arguments in
// the right registers.
public func repeatedLength(_ times: Int32) -> Int32 {
    return Int32(String(repeating: "ab", count: Int(times)).count)
}

public func wordCount(_ which: Int32) -> Int32 {
    let sentences = ["one two three", "a b c d", "single"]
    let i = Int(which)
    guard i >= 0 && i < sentences.count else { return -1 }
    return Int32(sentences[i].split(separator: " ").count)
}

public func sumTo(_ n: Int32) -> Int32 {
    return Int32((1...Int(n)).reduce(0, +))
}

public func sortedMiddle() -> Int32 {
    let xs = [9, 3, 7, 1, 5]
    return Int32(xs.sorted()[xs.count / 2])
}

public func dictionaryLookup(_ key: Int32) -> Int32 {
    let table: [Int32: Int32] = [1: 10, 2: 20, 3: 12]
    return table[key] ?? -1
}
