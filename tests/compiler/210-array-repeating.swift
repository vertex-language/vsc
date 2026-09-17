// [T](repeating:count:) makes an array of copies of one value -- integers,
// strings, a byte buffer -- and [T]() an empty one. A copy of the array
// grows on its own.
func main() -> Int32 {
    var total = 0
    let sevens = [Int](repeating: 7, count: 3)
    total += sevens.count + sevens[0] + sevens[2]

    let words = [String](repeating: "ab", count: 2)
    if words.count == 2 && words[1] == "ab" {
        total += 100
    }

    let buffer = [UInt8](repeating: 0, count: 4096)
    total += buffer.count / 64 + Int(buffer[4095])

    let empty = [Int]()
    if empty.isEmpty {
        total += 1000
    }

    let none = [String](repeating: "gone", count: 0)
    total += none.count

    var grown = sevens
    grown.append(8)
    total += grown.count * 10 + sevens.count + grown[3]

    return Int32(total % 251)
}
