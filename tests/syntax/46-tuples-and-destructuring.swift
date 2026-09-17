// §4.7 / §8 Tuples, Shuffling, and Destructuring

let simpleTuple: (Int, String) = (1, "one")
let labeledTuple: (x: Double, y: Double, z: Double) = (x: 1.0, y: 2.0, z: 3.0)

let nestedTuple: ((Int, Int), (String, (Bool, Double))) = (
    (10, 20),
    ("nested", (true, 3.14))
)

let (point, (_, (flag, ratio))) = nestedTuple
let (firstVal, secondVal) = point

let xCoord = labeledTuple.x
let yCoord = labeledTuple.y
let zCoord = labeledTuple.2

func swapValues<T>(_ a: inout T, _ b: inout T) {
    (a, b) = (b, a)
}

func returnsVoid() -> () {
    return ()
}

func testTupleExpressions() {
    var p1 = 100
    var p2 = 200
    swapValues(&p1, &p2)
    _ = (firstVal, secondVal, flag, ratio, xCoord, yCoord, zCoord, p1, p2)
}
