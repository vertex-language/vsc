// A class instance is a reference to storage.
final class Holder {
    var n: Int32 = 5
    func get() -> Int32 { return n }
}

func main() -> Int32 {
    let h = Holder()
    h.n = h.n + 37
    return h.get()
}
