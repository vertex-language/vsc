// A function takes what it is given and gives back a value.
func add(_ a: Int32, _ b: Int32) -> Int32 { return a + b }
func triple(_ n: Int32) -> Int32 { return n * 3 }

func main() -> Int32 {
    return add(triple(4), 8)
}
