// @_silgen_name says which symbol a declaration is, which is how a C
// function is called with no header to import. The names here are
// libc's, so both compilers reach the same code.
@_silgen_name("abs")
func c_abs(_ x: Int32) -> Int32

@_silgen_name("labs")
func c_labs(_ x: Int) -> Int

func main() -> Int32 {
    var n: Int32 = 0
    if c_abs(-5) == 5 { n += 1 }
    if c_abs(7) == 7 { n += 2 }
    if c_labs(-9) == 9 { n += 4 }
    if c_abs(0) == 0 { n += 8 }
    var total: Int32 = 0
    for i in -3...3 { total += c_abs(Int32(i)) }
    if total == 12 { n += 16 }
    return n
}
