// A struct that only a generic function uses, as the element of a local
// array: its metadata is there when the function is specialized.
struct Face {
    let at: Int
    let len: Int
}
func runs<T: Equatable>(_ xs: [T]) -> [String] {
    var faces: [Face] = []
    var i = 0
    while i < xs.count {
        var j = i + 1
        while j < xs.count && xs[j] == xs[i] { j += 1 }
        faces.append(Face(at: i, len: j - i))
        i = j
    }
    return faces.map { "\($0.at):\($0.len)" }
}
print(runs([1, 1, 2, 3, 3, 3]))
print(runs(["a", "b", "b"]))
