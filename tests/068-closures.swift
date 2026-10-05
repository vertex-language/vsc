// A closure expression in its full form.
let add = { (a: Int, b: Int) -> Int in
    return a + b
}
let greet = { (name: String) in "hi \(name)" }
let noArgs = { () -> Int in 42 }
print(add(3, 4), greet("bo"), noArgs())
