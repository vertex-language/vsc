// A default argument of an internal method calls an internal type's
// initializer.
struct Qualifiers {
    var isConst = false
    var precision = "mediump"
}
final class Parser {
    func typeSpecifier(_ name: String, qualifiers q: Qualifiers = Qualifiers()) -> String {
        (q.isConst ? "const " : "") + q.precision + " " + name
    }
}
let p = Parser()
print(p.typeSpecifier("vec4"))
print(p.typeSpecifier("float", qualifiers: Qualifiers(isConst: true, precision: "highp")))
