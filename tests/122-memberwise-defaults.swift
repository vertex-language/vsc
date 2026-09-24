// Stored properties with defaults may be left out of the memberwise call.
struct Config {
    var name: String
    var retries = 3
    var verbose = false
}
print(Config(name: "a"))
print(Config(name: "b", retries: 5))
print(Config(name: "c", verbose: true))
