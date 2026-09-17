// A type that conforms to a protocol, called on the concrete type.
protocol Answering {
    func answer() -> Int32
}

struct Yes: Answering {
    func answer() -> Int32 { return 42 }
}

func main() -> Int32 {
    let y = Yes()
    return y.answer()
}
