// init(exactly:) answers nil instead of trapping.
print(UInt8(exactly: 255) as Any, UInt8(exactly: 256) as Any, Int8(exactly: -129) as Any)
print(Int(exactly: 2.0) as Any, Int(exactly: 2.5) as Any)
