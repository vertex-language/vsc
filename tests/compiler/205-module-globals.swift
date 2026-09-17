// Variables declared at the top level are read and written from every
// function: a counter, a string, an array appended to, and a constant
// whose initializer is an expression.
var counter = 10
var label = "start"
let limit = 3 * 2
var names: [String] = ["a"]

func bump() {
    counter += 1
}

func rename(_ to: String) {
    label = to
    names.append(to)
}

func main() -> Int32 {
    bump()
    bump()
    rename("after")
    var n = counter + limit
    if label == "after" {
        n += 100
    }
    n += names.count * 1000
    return Int32(n % 251)
}
