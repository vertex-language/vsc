// Five levels, each overriding the one above.
class L0 { func v() -> Int32 { return 0 } }
class L1: L0 { override func v() -> Int32 { return 1 } }
class L2: L1 { override func v() -> Int32 { return 2 } }
class L3: L2 { override func v() -> Int32 { return 3 } }
class L4: L3 { override func v() -> Int32 { return 4 } }
func take(_ x: L0) -> Int32 { return x.v() }
func main() -> Int32 { return take(L0())*10000 + take(L1())*1000 + take(L2())*100 + take(L3())*10 + take(L4()) + 41 - 1234 }
