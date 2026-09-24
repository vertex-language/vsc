// 'mutating' is not valid on a class method. A class is a reference
// type: a method that changes a property changes the object, and the
// receiver needs no mutability of its own.
class Box {
    var v: Int32 = 0
    mutating func bump() { v = v + 1 }
}
