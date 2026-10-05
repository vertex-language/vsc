// Set: membership, insert and remove, and uniqueness.
var s: Set = [3, 1, 4, 1, 5, 9, 2, 6, 5]
print(s.count, s.contains(4), s.contains(7))
let (inserted, _) = s.insert(7)
let (again, _) = s.insert(7)
s.remove(1)
print(inserted, again, s.sorted())
print(Set("hello").sorted(), Set<Int>().isEmpty)
