// Arrays compare element by element, and an array literal compared with
// an array takes that array's element type.
func main() -> Int32 {
    var failures: Int32 = 0
    var unit: [UInt8] = []
    for b in "ms".utf8 {
        unit.append(b)
    }
    if !(unit == [109, 115]) { failures += 1 }
    if unit == [109] || unit != [109, 115] { failures += 1 }
    if !([109, 115] == unit) { failures += 1 }

    let words = ["a", "b"]
    if words != ["a", "b"] || words == ["a", "c"] { failures += 1 }
    let empty: [Int] = []
    if empty != [] { failures += 1 }
    let same = words
    if !(same == words) { failures += 1 }

    print(failures == 0 ? "ok" : "failed \(failures)")
    return failures
}
