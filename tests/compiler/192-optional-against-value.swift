// An optional against a plain value, both ways round, and a chain.
struct P { var x: Int32 }
func f(_ v: Int32?) -> Int32 { return v == 7 ? 1 : 0 }
func main() -> Int32 {
    var n: Int32 = 0
    let a: Int32? = 7
    let z: Int32? = nil
    if a == 7 { n += 1 }
    if 7 == a { n += 2 }
    if a != 9 { n += 4 }
    if z != 7 { n += 8 }
    if !(z == 7) { n += 16 }
    let p: P? = P(x: 3)
    if p?.x == 3 { n += 32 }
    let q: P? = nil
    if q?.x != 3 { n += 64 }
    n += f(7) + f(nil) * 10
    return n % 128
}
