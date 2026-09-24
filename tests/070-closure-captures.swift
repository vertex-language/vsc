// A closure captures variables by reference and keeps them alive.
func makeCounter() -> () -> Int {
    var count = 0
    return {
        count += 1
        return count
    }
}
let c1 = makeCounter()
let c2 = makeCounter()
print(c1(), c1(), c1(), c2())
var shared = 0
let add = { shared += 5 }
add(); add()
print(shared)
