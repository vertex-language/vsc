// Array(someSet), sorted to be printed.
let wanted: Set<String> = ["gralloc", "opengles", "qemud"]
let names = Array(wanted)
print(names.count, names.sorted())
let ns: Set = [3, 1, 2]
print(Array(ns).sorted(), Array(ns).reduce(0, +))
