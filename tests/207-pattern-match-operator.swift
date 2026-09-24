// A ~= of a program's own lets switch match values of new kinds.
struct Even {}
func ~= (_: Even, n: Int) -> Bool { n % 2 == 0 }
func ~= (prefix: String, s: String) -> Bool { s.hasPrefix(prefix) }
func kind(_ n: Int) -> String {
    switch n {
    case Even(): return "even"
    default: return "odd"
    }
}
func route(_ path: String) -> String {
    switch path {
    case "/api/": return "api"
    case "/static/": return "files"
    default: return "page"
    }
}
print(kind(4), kind(7), route("/api/users"), route("/static/a.png"), route("/home"))
