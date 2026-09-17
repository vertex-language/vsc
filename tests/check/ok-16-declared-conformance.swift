protocol Printable {
    func printMe()
}

struct Label: Printable {
    func printMe() {}
}

func take(_ p: Printable) {}

func use(_ l: Label) {
    take(l)
}
