// AnyObject holds any class instance, and identity survives the conversion.
class A {}
class B {}
let a = A()
let objects: [AnyObject] = [a, B(), a]
print(objects[0] === objects[2], objects[0] === objects[1])
print(objects.filter { $0 is A }.count, type(of: objects[1]) == B.self)
