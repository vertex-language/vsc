// A function declared inside another is a function.
func outer() -> Int32 {
    func double(_ n: Int32) -> Int32 { return n * 2 }
    func addOne(_ n: Int32) -> Int32 { return n + 1 }
    return double(addOne(20))
}

func other() -> Int32 {
    func double(_ n: Int32) -> Int32 { return n * 3 }
    return double(0)
}

func main() -> Int32 {
    return outer() + other()
}
