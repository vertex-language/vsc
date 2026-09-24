// defer runs at scope exit, the last one declared first.
func work() -> Int {
    defer { print("first defer") }
    defer { print("second defer") }
    print("body")
    return 1
}
print(work())
do {
    defer { print("leaving do") }
    print("in do")
}
