// Writing a computed property. It is a call to the property's setter:
// there is no storage to store into, and taking an address named a
// field the type does not have.
//
// Self is @inout on a value type -- the setter writes a property
// through it, and a copy would be written and dropped -- and the
// reference itself on a class, where the write goes through the
// reference and the object is what changes.
struct Temp {
    var raw: Int32

    var doubled: Int32 {
        get { return raw * 2 }
        set { raw = newValue / 2 }
    }

    // The value's name is the setter's to choose.
    var named: Int32 {
        get { return raw }
        set(v) { raw = v + 1 }
    }

    // set before get, which is where picking an accessor by position
    // rather than by keyword went wrong.
    var tripled: Int32 {
        set { raw = newValue / 3 }
        get { return raw * 3 }
    }
}

class Box {
    var n: Int32 = 0
    var doubled: Int32 {
        get { return n * 2 }
        set { n = newValue / 2 }
    }
}

func main() -> Int32 {
    var t = Temp(raw: 5)
    let read = t.doubled      // 10

    t.doubled = 20            // raw = 10
    let a = t.raw

    t.named = 6               // raw = 7
    let b = t.raw

    t.tripled = 27            // raw = 9
    let c = t.raw

    let box = Box()
    box.doubled = 14          // n = 7

    return read + a + b + c + box.n
}
