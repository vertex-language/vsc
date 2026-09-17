// Vertex functions with C entry points. Each is an ordinary function
// with a thunk beside it, which is what @_cdecl asks for -- so `fib`
// below is called by C through `vs_fib` and by `vs_sum` by its own
// name, and the two are the same body.
@_cdecl("vs_add")
func add(_ a: Int32, _ b: Int32) -> Int32 { return a + b }

@_cdecl("vs_fib")
func fib(_ n: Int32) -> Int32 {
    return n < 2 ? n : fib(n - 1) + fib(n - 2)
}

@_cdecl("vs_sum")
func sum(_ n: Int32) -> Int32 {
    var total: Int32 = 0
    for i in 1...n { total += add(Int32(i), 0) }
    return total
}
