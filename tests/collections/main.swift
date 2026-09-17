func main() -> Int32 {
    var a = [1, 2, 3]
    a.append(4)
    a[0] = 10
    a += [5, 6]
    a.insert(0, at: 1)
    let removed = a.remove(at: 2)
    let last = a.removeLast()
    print(a, removed, last, a.count, a.first, a.last, a.contains(10), a.contains(99))
    var names: [String] = []
    names.append("ada")
    names.append("grace, whose name is long enough for the heap")
    let copy = names
    names[0] = "alan"
    print(names, copy, names.first ?? "none")
    let empty: [Int] = []
    print(empty.first, empty.last, empty.isEmpty)
    var d = ["one": 1]
    d["two"] = 2
    d["three"] = 3
    d["one"] = nil
    print(d.count, d["two"], d["one"], d["missing"] ?? -1, d.isEmpty)
    let old = d.removeValue(forKey: "three")
    print(old, d.count, d)
    var scores: [Int: String] = [:]
    var i = 0
    while i < 1000 {
        scores[i] = "score \(i)"
        i += 1
    }
    var hits = 0
    i = 0
    while i < 1000 {
        if scores[i] != nil { hits += 1 }
        i += 1
    }
    print(scores.count, hits, scores[999]!)
    var s: Set<String> = ["x", "y"]
    s.insert("z")
    s.insert("x")
    print(s.count, s.contains("y"), s.contains("é"), s.remove("x"), s.remove("q"), s.count)
    let one: Set<Int> = [7]
    print(one, ["k": "v"], d.isEmpty)
    var lengths = 0
    var keys = 0
    for (key, value) in scores {
        lengths += value.count
        keys += key
    }
    for entry in d { print(entry.key, entry.value) }
    for x in s { lengths += x.count }
    for (k, v) in ["solo": 9] { print(k, v) }
    print(lengths, keys)
    var tally: [String: Int] = [:]
    for word in ["to", "be", "or", "not", "to", "be"] {
        tally[word, default: 0] += 1
    }
    var label: [Int: String] = [:]
    label[3, default: "three"] += "!"
    print(tally["to", default: 0], tally["be"]!, tally["or"]!, tally["xyz", default: -1], tally.count, label)
    a.removeAll()
    print(a, a.count)
    return 0
}
