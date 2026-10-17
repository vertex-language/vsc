// $0 in a closure passed to a range's first(where:) and other methods.
struct Group { let A: Int }
let xs = [Group(A: 1), Group(A: 5), Group(A: 9)]
let n = xs.count
print((0..<n).first(where: { xs[$0].A >= 4 }) ?? -1)
print((0..<n).first(where: { xs[$0].A > 100 }) ?? -1)
print((0...n).filter { $0 % 2 == 0 }, (1..<4).contains(where: { $0 * $0 == 9 }))
