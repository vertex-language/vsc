// Every integer type parses text in a radix, nil where it doesn't fit;
// and an empty collection literal in a ? : takes the other branch's type.
let mode: String? = "755"
print(mode.flatMap { UInt32($0, radix: 8) } as Any)
print(UInt8("ff", radix: 16) as Any, UInt8("100", radix: 16) as Any)
print(Int8("-80", radix: 16) as Any, Int8("80", radix: 16) as Any)
print(UInt64("ffffffffffffffff", radix: 16) as Any, UInt32("-1", radix: 10) as Any)
print(Int32("7fffffff", radix: 16) as Any, UInt16("zz", radix: 36) as Any, Int64("-z", radix: 36) as Any)

func read() throws -> [UInt8] { [1, 2] }
let link = false
let data = link ? [] : try! read()
print(data)
let names = link ? ["a"] : []
print(names.count)
let table = !link ? [:] : ["k": 1]
print(table.isEmpty)
