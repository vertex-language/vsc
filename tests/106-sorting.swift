// sort in place and sorted copies, by default and by a predicate.
var xs = [5, 2, 8, 1, 9, 3]
let asc = xs.sorted()
xs.sort(by: >)
print(asc, xs)
let words = ["banana", "kiwi", "apple", "fig"]
print(words.sorted { $0.count < $1.count || ($0.count == $1.count && $0 < $1) })
print([3, 1, 2].sorted().reversed() as [Int])
