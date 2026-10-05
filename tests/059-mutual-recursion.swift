// Two functions that call each other.
func isEven(_ n: Int) -> Bool { n == 0 ? true : isOdd(n - 1) }
func isOdd(_ n: Int) -> Bool { n == 0 ? false : isEven(n - 1) }
print(isEven(10), isOdd(10), isEven(7), isOdd(7))
