// Strings compare equal by canonical equivalence and order lexicographically.
let composed = "caf\u{E9}"
let decomposed = "cafe\u{301}"
print(composed == decomposed, composed.count, decomposed.count)
print("apple" < "banana", "Zebra" < "apple", "abc" < "abcd", "b" > "a")
print(["pear", "Fig", "apple"].sorted())
