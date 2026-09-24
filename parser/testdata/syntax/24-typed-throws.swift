// §6.4 / §4.6 Typed Throws (Swift 6.0)

enum MathError: Error {
    case divisionByZero
    case overflow
    case negativeSquareRoot
}

func safeDivide(_ a: Int, _ b: Int) throws(MathError) -> Int {
    if b == 0 {
        throw .divisionByZero
    }
    return a / b
}

protocol Calculator {
    func compute() throws(MathError) -> Double
}

struct BasicCalc: Calculator {
    func compute() throws(MathError) -> Double {
        return 42.0
    }
}

let typedClosure: (Int) throws(MathError) -> Int = { x in
    if x < 0 {
        throw MathError.negativeSquareRoot
    }
    return x * 2
}

func testTypedCatch() {
    do throws(MathError) {
        let result = try safeDivide(10, 2)
        print(result)
    } catch .divisionByZero {
        print("zero")
    } catch .overflow {
        print("overflow")
    } catch .negativeSquareRoot {
        print("neg")
    }
}
