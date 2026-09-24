// Subscripts on the type itself, and a static callAsFunction reached through a value.
enum Config {
    static var store: [String: String] = ["mode": "fast"]
    static subscript(key: String) -> String? {
        get { store[key] }
        set { store[key] = newValue }
    }
}
Config["level"] = "3"
print(Config["mode"] as Any, Config["level"] as Any, Config["none"] as Any)
Config["mode"] = nil
print(Config.store.count)
