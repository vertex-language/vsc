// Every integer type's min and max are constants of that type.
func main() -> Int32 {
    var failures: Int32 = 0
    let big = Int64.max
    if big != 9_223_372_036_854_775_807 { failures += 1 }
    if Int64.min + 1 != -9_223_372_036_854_775_807 { failures += 1 }
    if UInt64.max != 18_446_744_073_709_551_615 { failures += 1 }
    if Int8.min != -128 || Int8.max != 127 || UInt8.max != 255 { failures += 1 }
    if Int32.min != -2_147_483_648 || UInt16.max != 65_535 { failures += 1 }
    if Int.max != Int(Int64.max) || UInt32.min != 0 { failures += 1 }
    print(Int64.min, UInt8.max)
    print(failures == 0 ? "ok" : "failed")
    return failures
}
