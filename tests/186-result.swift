// Result holds a success or an error, and converts to and from throwing.
enum MathError: Error { case divideByZero }
func divide(_ a: Int, _ b: Int) -> Result<Int, MathError> {
    b == 0 ? .failure(.divideByZero) : .success(a / b)
}
for r in [divide(10, 2), divide(1, 0)] {
    switch r {
    case .success(let v): print("ok", v)
    case .failure(let e): print("failed", e)
    }
}
print(divide(9, 3).map { $0 * 10 }, (try? divide(8, 0).get()) as Any)
let wrapped = Result { () throws -> Int in try divide(7, 7).get() }
print(try wrapped.get())
