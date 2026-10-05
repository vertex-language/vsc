// An initializer a protocol extension declares, delegating to a required
// one.
protocol Made { init(n: Int) ; var n: Int { get } }
extension Made {
    init(twice m: Int) { self.init(n: m * 2) }
}
struct S: Made { var n: Int }
print(S(twice: 4).n)
func make<T: Made>(_: T.Type) -> T { T(twice: 5) }
print(make(S.self).n)
