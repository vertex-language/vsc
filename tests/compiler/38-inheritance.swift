// An override replaces the base's body for that slot.
class Animal {
    func legs() -> Int32 { return 4 }
}

class Bird: Animal {
    override func legs() -> Int32 { return 2 }
}

func count(_ a: Animal) -> Int32 { return a.legs() }

func main() -> Int32 {
    return count(Animal()) * 10 + count(Bird())
}
