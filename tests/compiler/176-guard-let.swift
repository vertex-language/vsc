// guard let binds for the rest of the function.
func doubled(_ v: Int32?) -> Int32 {
    guard let x = v else { return -1 }
    return x * 2
}
func main() -> Int32 {
    return doubled(21) + doubled(nil)
}
