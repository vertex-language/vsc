// A computed variable at the top level is a getter and nothing stored:
// every read calls it. A getter may be the shorthand body or an explicit
// get, may read a stored top-level constant, and may read another
// computed variable.
let prefix = "net"
let base = 16

var capacity: Int {
    return base * 4
}

var label: String {
    return prefix + "/tcp"
}

var doubled: Int {
    get {
        return capacity * 2
    }
}

func describe() -> String {
    return label + " \(capacity)"
}

func main() -> Int32 {
    var total = capacity + doubled
    if label == "net/tcp" {
        total += 100
    }
    if describe() == "net/tcp 64" {
        total += 1000
    }
    return Int32(total % 251)
}
