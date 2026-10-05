// Free functions overloaded by argument label alone.
func register(path: String, as name: String) -> String { "path \(path) as \(name)" }
func register(data: [UInt8], as name: String) -> String { "\(data.count) bytes as \(name)" }
print(register(path: "/a", as: "x"))
print(register(data: [1, 2, 3], as: "y"))
