// A nested function in a mutating method, called or not, capturing or not:
// the method's self is still self afterwards.
struct Parser {
    var i = 0
    mutating func f() {
        func g() -> Int { return 1 }
        i += g()
    }
    mutating func h() {
        func unused() -> Int { return 1 }
        i += 10
        self.i += 100
    }
    mutating func k() {
        func reads() -> Int { return i }
        let r = reads()
        i += r
    }
}
var p = Parser()
p.f()
p.h()
p.k()
print(p.i)
