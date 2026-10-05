// An optional whose payload is a struct holding a Bool beside another field.
struct Scancode { var code: UInt16; var extended: Bool }
func lookup(_ c: Int) -> Scancode? { c < 0 ? nil : Scancode(code: UInt16(c), extended: c > 0x7f) }
if let s = lookup(0x90) { print(s.code, s.extended) }
print(lookup(-1) == nil, lookup(5)!.extended)
