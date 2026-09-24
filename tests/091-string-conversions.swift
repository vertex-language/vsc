// Numbers to strings and back, in several radices.
print(String(255), String(255, radix: 16), String(255, radix: 2, uppercase: false), String(-8, radix: 8))
print(Int("123") as Any, Int("-7") as Any, Int("12a") as Any, Int("ff", radix: 16) as Any)
print(Double("2.5") as Any, Double("nope") as Any, String(1.5), String(true))
