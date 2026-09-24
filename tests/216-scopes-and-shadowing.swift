// Nested scopes: do blocks, shadowing, and names that end with their scope.
let x = 1
do {
    let x = x + 10
    print("inner", x)
    do {
        let x = "text"
        print("innermost", x)
    }
}
print("outer", x)
func f(_ x: Int) -> Int {
    var x = x
    x *= 2
    if x > 5 { let x = -x; return x }
    return x
}
print(f(2), f(4))
