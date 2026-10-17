// A typealias of a nested type, used to call the type's static methods.
enum binary {
    enum BigEndian {
        static func Uint16(_ b: [UInt8], from i: Int) -> UInt16 {
            UInt16(b[i]) << 8 | UInt16(b[i + 1])
        }
        static var name: String { "big" }
    }
}
typealias BE = binary.BigEndian
print(BE.Uint16([0x12, 0x34, 0x56], from: 1), BE.name)
print(binary.BigEndian.Uint16([1, 2], from: 0))
