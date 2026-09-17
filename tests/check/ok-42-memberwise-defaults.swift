struct Config {
    var name: String = "default"
    var retries: Int = 3
    var verbose: Bool = false
}
func use() -> Int {
    let c = Config(name: "x", retries: 5, verbose: true)
    return c.retries
}
