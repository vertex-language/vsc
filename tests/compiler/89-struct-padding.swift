// Fields of different widths, each at the offset its alignment wants.
struct Mixed {
    var flag: Bool
    var big: Int64
    var small: Int8
    var mid: Int32
}

func main() -> Int32 {
    var m = Mixed(flag: true, big: 1000, small: 7, mid: 20)
    m.small = 9
    m.flag = false
    if m.flag { return 91 }
    if m.big != 1000 { return 92 }
    if m.small != 9 { return 93 }
    if m.mid != 20 { return 94 }
    return 42
}
