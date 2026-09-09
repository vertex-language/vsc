// A parameter the caller may leave out.
//
// Swift evaluates the default at the call, which is why a client's
// object file references the function and nothing else: there is no
// argument sitting in the callee waiting to be filled in. So a call
// with fewer arguments than parameters is not a call with fewer
// arguments -- the caller supplies the rest.
//
// Passing what was written and stopping there leaves the callee
// reading a register nobody wrote. That is what this used to do, and
// nothing said so: the VIL applied one argument to a function of two.
struct Vec {
    var x: Int32
    var y: Int32
    func scaled(by n: Int32 = 2) -> Vec { return Vec(x: x * n, y: y * n) }
}

func defaulted(_ n: Int32, by k: Int32 = 3) -> Int32 { return n + k }
func twoDefaults(_ n: Int32, a: Int32 = 1, b: Int32 = 2) -> Int32 { return n + a + b }

// A defaulted parameter with an ordinary one after it, which is where
// filling them in out of order would show.
func middle(_ n: Int32, a: Int32 = 10, _ m: Int32) -> Int32 { return n * 100 + a * 10 + m }

func main() -> Int32 {
    if defaulted(39) != 42 { return 91 }
    if defaulted(39, by: 1) != 40 { return 92 }

    if twoDefaults(39) != 42 { return 93 }
    if twoDefaults(39, a: 2) != 43 { return 94 }
    if twoDefaults(39, b: 0) != 40 { return 95 }
    if twoDefaults(30, a: 5, b: 7) != 42 { return 96 }

    if middle(1, 2) != 112 { return 97 }
    if middle(1, a: 3, 2) != 132 { return 98 }

    // A method's default, which is the same rule with a receiver.
    let v = Vec(x: 21, y: 0)
    if v.scaled().x != 42 { return 99 }
    if v.scaled(by: 3).x != 63 { return 100 }

    return defaulted(39)
}
