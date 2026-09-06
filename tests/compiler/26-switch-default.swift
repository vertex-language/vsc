// The default takes what no case named.
func main() -> Int32 {
    var total: Int32 = 0
    for i in 0..<6 {
        switch i {
        case 0: total = total + 1
        case 3: total = total + 10
        default: total = total + 100
        }
    }
    return total
}
