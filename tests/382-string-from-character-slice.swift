// A String made from part of an array of Characters is those
// Characters, not a description of the slice.
let cs = Array("x${NAME}y")
let name = String(cs[3..<7])
print(name, name.count, name == "NAME")
let env = ["NAME": "vertex"]
print(env[name] ?? "missing", String(cs[0..<1]) + String(cs[8...]))
print(String(cs[2..<2]).isEmpty)
