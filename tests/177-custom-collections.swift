// A Collection of a program's own gets indices, count, slicing and the algorithms.
struct Ring: RandomAccessCollection {
    let base: [Int]
    let offset: Int
    var startIndex: Int { 0 }
    var endIndex: Int { base.count }
    subscript(i: Int) -> Int { base[(i + offset) % base.count] }
}
let r = Ring(base: [1, 2, 3, 4, 5], offset: 2)
print(Array(r), r.count, r.first as Any, r.last as Any, r[1])
print(r.firstIndex(of: 1) as Any, r.contains(4), Array(r.reversed()), Array(r.dropFirst(3)))
