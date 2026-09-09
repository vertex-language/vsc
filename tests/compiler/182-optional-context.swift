// An operator written where an optional is wanted works on the
// wrapped type, and the value is injected above it.
func f(_ v: Int32?) -> Int32 {
    guard let x = v else { return 100 }
    return x
}
func main() -> Int32 {
    let a: Int32? = 1 + 2
    let b: Int32? = -7
    let c: Int32? = 4 * 2 - 1
    var d: Int32? = nil
    d = 6 / 2
    return f(a) + f(b) + f(c) + f(d) + f(-1) + f(nil)
}
