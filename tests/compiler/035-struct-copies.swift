// Assigning a struct copies it: writing one does not change the other.
struct Box {
    var n: Int32
}

func main() -> Int32 {
    var a = Box(n: 1)
    var b = a
    b.n = 99
    if a.n != 1 { return 91 }
    if b.n != 99 { return 92 }
    return 42
}
