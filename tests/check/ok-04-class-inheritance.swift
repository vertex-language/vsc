class Base {
    var value = 0
}
class Derived: Base {
    var extra: String = ""
}
func use(_ d: Derived) -> Int {
    return d.value
}
