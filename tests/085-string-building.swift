// Concatenation with + and +=, append, and repeating.
var s = "ab"
s += "cd"
s = s + "-" + s
s.append("!")
s.append(contentsOf: "??")
print(s, String(repeating: "xy", count: 3), s.isEmpty, "".isEmpty)
