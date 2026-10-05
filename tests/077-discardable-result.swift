// A result may be ignored with _ =, or by @discardableResult.
@discardableResult
func record(_ s: String) -> Int {
    print("recorded", s)
    return s.count
}
func length(_ s: String) -> Int { s.count }
record("a")
_ = length("ignored")
print(record("bcd"))
