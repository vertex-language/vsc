// A call reaches the declaration its arguments chose.
func f(_ n: Int32) -> Int32 { return n }
func f(_ b: Bool) -> Int32 { return b ? 100 : 200 }
func f(_ a: Int32, _ b: Int32) -> Int32 { return a * b }

func g(_ n: Int64) -> Int32 { return 7 }
func g(_ n: Int8) -> Int32 { return 9 }

func main() -> Int32 {
    let a = f(1)
    let b = f(true)
    let c = f(2, 3)
    let d = g(Int64(1))
    let e = g(Int8(1))
    return a + b + c + d + e - 81
}
