// Array.reverse() in place, beside reversed().
var path = [1, 2, 3, 4]
path.reverse()
print(path)
var names = ["a", "b", "c"]
names.reverse()
print(names, Array(names.reversed()))
var empty: [Int] = []
empty.reverse()
print(empty)
