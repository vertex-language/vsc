// Union, intersection, differences, and subset tests.
let a: Set = [1, 2, 3, 4]
let b: Set = [3, 4, 5]
print(a.union(b).sorted(), a.intersection(b).sorted())
print(a.subtracting(b).sorted(), a.symmetricDifference(b).sorted())
print(Set([3, 4]).isSubset(of: a), a.isSuperset(of: b), a.isDisjoint(with: [9]))
