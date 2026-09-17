func describe(_ x: Int) -> String { return "int" }
func describe(_ x: String) -> String { return "string" }
func describe(_ x: Int, _ y: Int) -> String { return "two" }

func label(a: Int) -> Int { return a }
func label(b: Int) -> Int { return b }

func use() -> String {
    _ = label(a: 1)
    _ = label(b: 2)
    return describe(1) + describe("x") + describe(1, 2)
}
