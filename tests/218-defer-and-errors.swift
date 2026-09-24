// defer runs on every exit: a return, a throw, and a caught error in between.
struct Stop: Error {}
func step(_ n: Int) throws -> Int {
    print("enter", n)
    defer { print("leave", n) }
    if n == 2 { throw Stop() }
    if n == 3 { return -1 }
    return try step(n + 1) + 1
}
do { print(try step(0)) } catch { print("stopped") }
print((try? step(3)) as Any)
