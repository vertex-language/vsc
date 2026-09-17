// A property that is a function.
//
// A computed property has no storage. `v.magnitude` is a call to a
// getter that takes the receiver and hands back a value, and swiftc
// names it `$s...9magnitudes5Int32Vvg` -- v for a variable, g for its
// getter. There is no field to read.
//
// The mistake underneath that one is worse than reading the wrong
// bytes. A computed property counted as storage changes what the
// type's bytes are: `Vec` here would be sixteen bytes rather than
// eight, so it would cross a call in two registers where swiftc
// passes one, and every field after it would be at the wrong offset.
// A static property is the same mistake for the same reason -- it is
// the type's storage, not an instance's.
struct Vec {
    var x: Int32
    var y: Int32

    var magnitude: Int32 { return x * x + y * y }
    var doubled: Vec { return Vec(x: x * 2, y: y * 2) }

    static let unit: Int32 = 1
}

// A computed property reached from outside the type, and one whose
// value is itself a struct.
func lengthOf(_ v: Vec) -> Int32 { return v.magnitude }

// Layout is the thing this holds: a struct with a computed property
// in the middle of its stored ones has to be laid out as though the
// computed one were not there.
struct Mixed {
    var a: Int32
    var half: Int32 { return a / 2 }
    var b: Int32
    var sum: Int32 { return a + b }
}
func mixedSum(_ m: Mixed) -> Int32 { return m.a + m.b }

func main() -> Int32 {
    let v = Vec(x: 6, y: 1)
    if v.magnitude != 37 { return 91 }
    if lengthOf(v) != 37 { return 92 }
    if v.doubled.x != 12 { return 93 }

    let m = Mixed(a: 20, b: 22)
    if m.a != 20 { return 94 }
    if m.b != 22 { return 95 }
    if m.half != 10 { return 96 }
    if m.sum != 42 { return 97 }
    if mixedSum(m) != 42 { return 98 }

    return m.sum
}
