// A Substring split: lines of a String, each split into fields, with
// maxSplits and empty pieces kept; the pieces share the String's base.
let text = "a b  c\nd,e f\n\n  g"
for line in text.split(separator: "\n") {
    let parts = line.split(separator: " ")
    print(parts.count, parts.map { String($0) }, line.split(separator: " ", maxSplits: 1, omittingEmptySubsequences: false).map { String($0) })
}
let mid = text.split(separator: "\n")[1]
print(mid.split(separator: ",").map { String($0) }, mid.split(separator: ",")[1].split(separator: " ").count)
