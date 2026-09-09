// A label is part of the call, not of the type.
func move(from a: Int32, to b: Int32) -> Int32 { return b - a }

func main() -> Int32 {
    return move(from: 10, to: 42)
}
