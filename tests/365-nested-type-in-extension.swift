// A nested type declared inside an extension, used by the extension's methods.
struct Compiler { var count = 0 }
extension Compiler {
    struct Reference { let slot: Int }
    mutating func next() -> Reference {
        count += 1
        return Reference(slot: count)
    }
}
var c = Compiler()
_ = c.next()
let r: Compiler.Reference = c.next()
print(r.slot)
