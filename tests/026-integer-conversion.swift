// Converting between integer types where the value fits.
let small: Int8 = -5
let wide = Int64(small)
let unsigned = UInt32(200)
let back = Int8(unsigned - 100)
print(wide, unsigned, back, Int(UInt16.max), UInt8(Int32(255)))
