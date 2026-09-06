// A receiver goes last, so it lands past the arguments.
final class Adder {
    var base: Int32 = 100

    func sum(_ a: Int32, _ b: Int32, _ c: Int32, _ d: Int32,
             _ e: Int32, _ f: Int32, _ g: Int32, _ h: Int32) -> Int32 {
        return base + a + b + c + d + e + f + g + h
    }
}

func main() -> Int32 {
    let x = Adder()
    return x.sum(1, 2, 3, 4, 5, 6, 7, 8) - 100
}
