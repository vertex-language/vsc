// An implicit member in one arm of ?: takes the other arm's type.
enum GLError { case none, invalidEnum, outOfMemory }
struct Context { var PendingError = GLError.invalidEnum }
let g = Context()
for debug in [true, false] {
    let before = debug ? g.PendingError : .none
    print(before)
}
let n: Int? = nil
let k = n != nil ? n! : .zero
print(k)
