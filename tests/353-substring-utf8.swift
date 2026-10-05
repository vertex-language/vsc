// A Substring's utf8 view.
let line = "key=välue;rest"
let part = line.split(separator: ";")[0]
print(part.utf8.count, Array(part.utf8.prefix(4)), part.utf8.first!, part.utf8.contains(61))
