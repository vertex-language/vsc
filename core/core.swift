// The built-in module.
//
// What every program can see without importing anything: the
// operators on the primitive types, declared here rather than known
// to the compiler, because an operator in Swift is a function and a
// call to one should resolve the way every other call does.
//
// The declarations have no bodies. Their implementations are machine
// instructions — `+` on Int is an add — and core.go says which, the
// way Swift's own `+` is a @_transparent wrapper around
// Builtin.sadd_with_overflow. A body would be a lie about where the
// work happens.
//
// The precedence groups these operators belong to are built into the
// analyzer today rather than declared here; see core.go.

// ---- arithmetic ----

// Int is written first in every group on purpose: an untyped
// literal is assignable to all of them, and overload resolution
// takes the first candidate that fits, so `1 + 2` is an Int.

func + (lhs: Int, rhs: Int) -> Int
func - (lhs: Int, rhs: Int) -> Int
func * (lhs: Int, rhs: Int) -> Int
func / (lhs: Int, rhs: Int) -> Int
func % (lhs: Int, rhs: Int) -> Int

func + (lhs: Int8, rhs: Int8) -> Int8
func - (lhs: Int8, rhs: Int8) -> Int8
func * (lhs: Int8, rhs: Int8) -> Int8
func / (lhs: Int8, rhs: Int8) -> Int8
func % (lhs: Int8, rhs: Int8) -> Int8

func + (lhs: Int16, rhs: Int16) -> Int16
func - (lhs: Int16, rhs: Int16) -> Int16
func * (lhs: Int16, rhs: Int16) -> Int16
func / (lhs: Int16, rhs: Int16) -> Int16
func % (lhs: Int16, rhs: Int16) -> Int16

func + (lhs: Int32, rhs: Int32) -> Int32
func - (lhs: Int32, rhs: Int32) -> Int32
func * (lhs: Int32, rhs: Int32) -> Int32
func / (lhs: Int32, rhs: Int32) -> Int32
func % (lhs: Int32, rhs: Int32) -> Int32

func + (lhs: Int64, rhs: Int64) -> Int64
func - (lhs: Int64, rhs: Int64) -> Int64
func * (lhs: Int64, rhs: Int64) -> Int64
func / (lhs: Int64, rhs: Int64) -> Int64
func % (lhs: Int64, rhs: Int64) -> Int64

func + (lhs: UInt, rhs: UInt) -> UInt
func - (lhs: UInt, rhs: UInt) -> UInt
func * (lhs: UInt, rhs: UInt) -> UInt
func / (lhs: UInt, rhs: UInt) -> UInt
func % (lhs: UInt, rhs: UInt) -> UInt

func + (lhs: UInt8, rhs: UInt8) -> UInt8
func - (lhs: UInt8, rhs: UInt8) -> UInt8
func * (lhs: UInt8, rhs: UInt8) -> UInt8
func / (lhs: UInt8, rhs: UInt8) -> UInt8
func % (lhs: UInt8, rhs: UInt8) -> UInt8

func + (lhs: UInt16, rhs: UInt16) -> UInt16
func - (lhs: UInt16, rhs: UInt16) -> UInt16
func * (lhs: UInt16, rhs: UInt16) -> UInt16
func / (lhs: UInt16, rhs: UInt16) -> UInt16
func % (lhs: UInt16, rhs: UInt16) -> UInt16

func + (lhs: UInt32, rhs: UInt32) -> UInt32
func - (lhs: UInt32, rhs: UInt32) -> UInt32
func * (lhs: UInt32, rhs: UInt32) -> UInt32
func / (lhs: UInt32, rhs: UInt32) -> UInt32
func % (lhs: UInt32, rhs: UInt32) -> UInt32

func + (lhs: UInt64, rhs: UInt64) -> UInt64
func - (lhs: UInt64, rhs: UInt64) -> UInt64
func * (lhs: UInt64, rhs: UInt64) -> UInt64
func / (lhs: UInt64, rhs: UInt64) -> UInt64
func % (lhs: UInt64, rhs: UInt64) -> UInt64

func + (lhs: Float, rhs: Float) -> Float
func - (lhs: Float, rhs: Float) -> Float
func * (lhs: Float, rhs: Float) -> Float
func / (lhs: Float, rhs: Float) -> Float

func + (lhs: Double, rhs: Double) -> Double
func - (lhs: Double, rhs: Double) -> Double
func * (lhs: Double, rhs: Double) -> Double
func / (lhs: Double, rhs: Double) -> Double

// Concatenation is the one `+` that allocates.
func + (lhs: String, rhs: String) -> String

// ---- comparison ----

// Every ordering on every primitive, rather than the handful that
// happened to be needed. An operator missing here is not a
// diagnostic about the program -- it reads as `cannot lower this
// expression`, which is the compiler's own gap wearing the
// language's clothes.

func == (lhs: Int, rhs: Int) -> Bool
func != (lhs: Int, rhs: Int) -> Bool
func < (lhs: Int, rhs: Int) -> Bool
func <= (lhs: Int, rhs: Int) -> Bool
func > (lhs: Int, rhs: Int) -> Bool
func >= (lhs: Int, rhs: Int) -> Bool

func == (lhs: Int8, rhs: Int8) -> Bool
func != (lhs: Int8, rhs: Int8) -> Bool
func < (lhs: Int8, rhs: Int8) -> Bool
func <= (lhs: Int8, rhs: Int8) -> Bool
func > (lhs: Int8, rhs: Int8) -> Bool
func >= (lhs: Int8, rhs: Int8) -> Bool

func == (lhs: Int16, rhs: Int16) -> Bool
func != (lhs: Int16, rhs: Int16) -> Bool
func < (lhs: Int16, rhs: Int16) -> Bool
func <= (lhs: Int16, rhs: Int16) -> Bool
func > (lhs: Int16, rhs: Int16) -> Bool
func >= (lhs: Int16, rhs: Int16) -> Bool

func == (lhs: Int32, rhs: Int32) -> Bool
func != (lhs: Int32, rhs: Int32) -> Bool
func < (lhs: Int32, rhs: Int32) -> Bool
func <= (lhs: Int32, rhs: Int32) -> Bool
func > (lhs: Int32, rhs: Int32) -> Bool
func >= (lhs: Int32, rhs: Int32) -> Bool

func == (lhs: Int64, rhs: Int64) -> Bool
func != (lhs: Int64, rhs: Int64) -> Bool
func < (lhs: Int64, rhs: Int64) -> Bool
func <= (lhs: Int64, rhs: Int64) -> Bool
func > (lhs: Int64, rhs: Int64) -> Bool
func >= (lhs: Int64, rhs: Int64) -> Bool

func == (lhs: UInt, rhs: UInt) -> Bool
func != (lhs: UInt, rhs: UInt) -> Bool
func < (lhs: UInt, rhs: UInt) -> Bool
func <= (lhs: UInt, rhs: UInt) -> Bool
func > (lhs: UInt, rhs: UInt) -> Bool
func >= (lhs: UInt, rhs: UInt) -> Bool

func == (lhs: UInt8, rhs: UInt8) -> Bool
func != (lhs: UInt8, rhs: UInt8) -> Bool
func < (lhs: UInt8, rhs: UInt8) -> Bool
func <= (lhs: UInt8, rhs: UInt8) -> Bool
func > (lhs: UInt8, rhs: UInt8) -> Bool
func >= (lhs: UInt8, rhs: UInt8) -> Bool

func == (lhs: UInt16, rhs: UInt16) -> Bool
func != (lhs: UInt16, rhs: UInt16) -> Bool
func < (lhs: UInt16, rhs: UInt16) -> Bool
func <= (lhs: UInt16, rhs: UInt16) -> Bool
func > (lhs: UInt16, rhs: UInt16) -> Bool
func >= (lhs: UInt16, rhs: UInt16) -> Bool

func == (lhs: UInt32, rhs: UInt32) -> Bool
func != (lhs: UInt32, rhs: UInt32) -> Bool
func < (lhs: UInt32, rhs: UInt32) -> Bool
func <= (lhs: UInt32, rhs: UInt32) -> Bool
func > (lhs: UInt32, rhs: UInt32) -> Bool
func >= (lhs: UInt32, rhs: UInt32) -> Bool

func == (lhs: UInt64, rhs: UInt64) -> Bool
func != (lhs: UInt64, rhs: UInt64) -> Bool
func < (lhs: UInt64, rhs: UInt64) -> Bool
func <= (lhs: UInt64, rhs: UInt64) -> Bool
func > (lhs: UInt64, rhs: UInt64) -> Bool
func >= (lhs: UInt64, rhs: UInt64) -> Bool

func == (lhs: Float, rhs: Float) -> Bool
func != (lhs: Float, rhs: Float) -> Bool
func < (lhs: Float, rhs: Float) -> Bool
func <= (lhs: Float, rhs: Float) -> Bool
func > (lhs: Float, rhs: Float) -> Bool
func >= (lhs: Float, rhs: Float) -> Bool

func == (lhs: Double, rhs: Double) -> Bool
func != (lhs: Double, rhs: Double) -> Bool
func < (lhs: Double, rhs: Double) -> Bool
func <= (lhs: Double, rhs: Double) -> Bool
func > (lhs: Double, rhs: Double) -> Bool
func >= (lhs: Double, rhs: Double) -> Bool

func == (lhs: Bool, rhs: Bool) -> Bool
func != (lhs: Bool, rhs: Bool) -> Bool

func == (lhs: String, rhs: String) -> Bool
func != (lhs: String, rhs: String) -> Bool
func < (lhs: String, rhs: String) -> Bool

func == (lhs: Character, rhs: Character) -> Bool
func != (lhs: Character, rhs: Character) -> Bool
// ---- bitwise ----

// Swift declares these once on FixedWidthInteger and lets the
// generic system find them. There is no generic system reaching this
// far yet, so they are written out per type, which is the same set of
// functions said the long way.

func & (lhs: Int, rhs: Int) -> Int
func | (lhs: Int, rhs: Int) -> Int
func ^ (lhs: Int, rhs: Int) -> Int
func << (lhs: Int, rhs: Int) -> Int
func >> (lhs: Int, rhs: Int) -> Int

func & (lhs: Int8, rhs: Int8) -> Int8
func | (lhs: Int8, rhs: Int8) -> Int8
func ^ (lhs: Int8, rhs: Int8) -> Int8
func << (lhs: Int8, rhs: Int8) -> Int8
func >> (lhs: Int8, rhs: Int8) -> Int8

func & (lhs: Int16, rhs: Int16) -> Int16
func | (lhs: Int16, rhs: Int16) -> Int16
func ^ (lhs: Int16, rhs: Int16) -> Int16
func << (lhs: Int16, rhs: Int16) -> Int16
func >> (lhs: Int16, rhs: Int16) -> Int16

func & (lhs: Int32, rhs: Int32) -> Int32
func | (lhs: Int32, rhs: Int32) -> Int32
func ^ (lhs: Int32, rhs: Int32) -> Int32
func << (lhs: Int32, rhs: Int32) -> Int32
func >> (lhs: Int32, rhs: Int32) -> Int32

func & (lhs: Int64, rhs: Int64) -> Int64
func | (lhs: Int64, rhs: Int64) -> Int64
func ^ (lhs: Int64, rhs: Int64) -> Int64
func << (lhs: Int64, rhs: Int64) -> Int64
func >> (lhs: Int64, rhs: Int64) -> Int64

func & (lhs: UInt, rhs: UInt) -> UInt
func | (lhs: UInt, rhs: UInt) -> UInt
func ^ (lhs: UInt, rhs: UInt) -> UInt
func << (lhs: UInt, rhs: UInt) -> UInt
func >> (lhs: UInt, rhs: UInt) -> UInt

func & (lhs: UInt8, rhs: UInt8) -> UInt8
func | (lhs: UInt8, rhs: UInt8) -> UInt8
func ^ (lhs: UInt8, rhs: UInt8) -> UInt8
func << (lhs: UInt8, rhs: UInt8) -> UInt8
func >> (lhs: UInt8, rhs: UInt8) -> UInt8

func & (lhs: UInt16, rhs: UInt16) -> UInt16
func | (lhs: UInt16, rhs: UInt16) -> UInt16
func ^ (lhs: UInt16, rhs: UInt16) -> UInt16
func << (lhs: UInt16, rhs: UInt16) -> UInt16
func >> (lhs: UInt16, rhs: UInt16) -> UInt16

func & (lhs: UInt32, rhs: UInt32) -> UInt32
func | (lhs: UInt32, rhs: UInt32) -> UInt32
func ^ (lhs: UInt32, rhs: UInt32) -> UInt32
func << (lhs: UInt32, rhs: UInt32) -> UInt32
func >> (lhs: UInt32, rhs: UInt32) -> UInt32

func & (lhs: UInt64, rhs: UInt64) -> UInt64
func | (lhs: UInt64, rhs: UInt64) -> UInt64
func ^ (lhs: UInt64, rhs: UInt64) -> UInt64
func << (lhs: UInt64, rhs: UInt64) -> UInt64
func >> (lhs: UInt64, rhs: UInt64) -> UInt64

// ---- overflow ----

// The masking operators. `&+` is `+` with the overflow dropped
// rather than trapped on, and `&<<` is a shift whose count is taken
// modulo the width. Same set, said the long way, for the same
// reason the bitwise operators above are.

func &+ (lhs: Int, rhs: Int) -> Int
func &- (lhs: Int, rhs: Int) -> Int
func &* (lhs: Int, rhs: Int) -> Int
func &<< (lhs: Int, rhs: Int) -> Int
func &>> (lhs: Int, rhs: Int) -> Int

func &+ (lhs: Int8, rhs: Int8) -> Int8
func &- (lhs: Int8, rhs: Int8) -> Int8
func &* (lhs: Int8, rhs: Int8) -> Int8
func &<< (lhs: Int8, rhs: Int8) -> Int8
func &>> (lhs: Int8, rhs: Int8) -> Int8

func &+ (lhs: Int16, rhs: Int16) -> Int16
func &- (lhs: Int16, rhs: Int16) -> Int16
func &* (lhs: Int16, rhs: Int16) -> Int16
func &<< (lhs: Int16, rhs: Int16) -> Int16
func &>> (lhs: Int16, rhs: Int16) -> Int16

func &+ (lhs: Int32, rhs: Int32) -> Int32
func &- (lhs: Int32, rhs: Int32) -> Int32
func &* (lhs: Int32, rhs: Int32) -> Int32
func &<< (lhs: Int32, rhs: Int32) -> Int32
func &>> (lhs: Int32, rhs: Int32) -> Int32

func &+ (lhs: Int64, rhs: Int64) -> Int64
func &- (lhs: Int64, rhs: Int64) -> Int64
func &* (lhs: Int64, rhs: Int64) -> Int64
func &<< (lhs: Int64, rhs: Int64) -> Int64
func &>> (lhs: Int64, rhs: Int64) -> Int64

func &+ (lhs: UInt, rhs: UInt) -> UInt
func &- (lhs: UInt, rhs: UInt) -> UInt
func &* (lhs: UInt, rhs: UInt) -> UInt
func &<< (lhs: UInt, rhs: UInt) -> UInt
func &>> (lhs: UInt, rhs: UInt) -> UInt

func &+ (lhs: UInt8, rhs: UInt8) -> UInt8
func &- (lhs: UInt8, rhs: UInt8) -> UInt8
func &* (lhs: UInt8, rhs: UInt8) -> UInt8
func &<< (lhs: UInt8, rhs: UInt8) -> UInt8
func &>> (lhs: UInt8, rhs: UInt8) -> UInt8

func &+ (lhs: UInt16, rhs: UInt16) -> UInt16
func &- (lhs: UInt16, rhs: UInt16) -> UInt16
func &* (lhs: UInt16, rhs: UInt16) -> UInt16
func &<< (lhs: UInt16, rhs: UInt16) -> UInt16
func &>> (lhs: UInt16, rhs: UInt16) -> UInt16

func &+ (lhs: UInt32, rhs: UInt32) -> UInt32
func &- (lhs: UInt32, rhs: UInt32) -> UInt32
func &* (lhs: UInt32, rhs: UInt32) -> UInt32
func &<< (lhs: UInt32, rhs: UInt32) -> UInt32
func &>> (lhs: UInt32, rhs: UInt32) -> UInt32

func &+ (lhs: UInt64, rhs: UInt64) -> UInt64
func &- (lhs: UInt64, rhs: UInt64) -> UInt64
func &* (lhs: UInt64, rhs: UInt64) -> UInt64
func &<< (lhs: UInt64, rhs: UInt64) -> UInt64
func &>> (lhs: UInt64, rhs: UInt64) -> UInt64
// ---- prefix ----

prefix func ~ (operand: Int) -> Int
prefix func ~ (operand: Int8) -> Int8
prefix func ~ (operand: Int16) -> Int16
prefix func ~ (operand: Int32) -> Int32
prefix func ~ (operand: Int64) -> Int64
prefix func ~ (operand: UInt) -> UInt
prefix func ~ (operand: UInt8) -> UInt8
prefix func ~ (operand: UInt16) -> UInt16
prefix func ~ (operand: UInt32) -> UInt32
prefix func ~ (operand: UInt64) -> UInt64
// ---- logical ----

func && (lhs: Bool, rhs: Bool) -> Bool
func || (lhs: Bool, rhs: Bool) -> Bool

// ---- prefix ----

prefix func - (operand: Int) -> Int
prefix func - (operand: Int32) -> Int32
prefix func - (operand: Int64) -> Int64
prefix func - (operand: Float) -> Float
prefix func - (operand: Double) -> Double

prefix func + (operand: Int) -> Int
prefix func + (operand: Double) -> Double

prefix func ! (operand: Bool) -> Bool

// ---- output ----

// The one function every program writes first.
//
// Its body is in the runtime rather than in an instruction, so unlike
// everything above it this is a call: `print("hi")` reaches
// vertex_print, which is the symbol this declaration names. The
// variadic list becomes an array of
// `Any`, each element carrying the metadata that says what is in it,
// and the two defaults are evaluated at the call the way Swift
// evaluates any default.
// The program's arguments, its own path first. The property is the
// runtime's; see core.LowerStaticMember.
enum CommandLine {}

// readLine: a line of standard input, or nil where there is none left.
@_silgen_name("vertex_read_line")
func readLine(strippingNewline: Bool = true) -> String?

@_silgen_name("vertex_print")
func print(_ items: Any..., separator: String = " ", terminator: String = "\n")

// ---- concurrency ----

// A task: a function running on a stack of its own, which the runtime's
// executor switches to, and away from wherever it waits. `Task { ... }`
// starts one, which runs when the task that started it next waits. See
// stdlib/runtime/task.cpp.
//
// A Task value is a counted handle to its task, which `value` waits on:
// the task's operation returns nothing, so its value is that it finished.
final class TaskHandle {}

struct Task {
    let handle: TaskHandle
    // Where the operation's result is kept, for value to read: no bytes,
    // for an operation returning nothing.
    let result: TaskHandle

    @_silgen_name("vertex_task_start")
    init(operation: @escaping () async -> Void)

    // A task started on the worker pool whatever started it, as Swift's
    // Task.detached is; `Task { }` starts where it is made.
    @_silgen_name("vertex_task_start_detached")
    static func detached(operation: @escaping () async -> Void) -> Task

    @_silgen_name("vertex_task_sleep")
    static func sleep(nanoseconds duration: UInt64) async throws

    @_silgen_name("vertex_task_yield")
    static func yield() async
}

// The main actor: Thread 0, which hosts the window system and runs what
// is marked @MainActor. `await MainActor.run { … }` runs the closure there
// and comes back; `MainActor.assumeIsolated { … }` runs it here, where the
// caller knows it is already there (see also the @MainActor attribute).
//
// The hops are the runtime's: 0 is the main executor, 1 the pool, 2 the
// executor the task calls home.
enum MainActor {
    @_silgen_name("vertex_task_hop")
    static func hop(_ to: UInt64) async

    static func run(_ body: @MainActor () async -> Void) async
    static func assumeIsolated(_ body: @MainActor () -> Void)
}

// The names Swift gives C's types, which an interface imported from a C
// header writes: `char` is CChar, whose signedness is the platform's and
// is signed on every target this compiler builds for.
public typealias CChar = Int8
public typealias CSignedChar = Int8
public typealias CUnsignedChar = UInt8
public typealias CShort = Int16
public typealias CUnsignedShort = UInt16
public typealias CInt = Int32
public typealias CUnsignedInt = UInt32
public typealias CLongLong = Int64
public typealias CUnsignedLongLong = UInt64
public typealias CFloat = Float
public typealias CDouble = Double
public typealias CBool = Bool

// A value that hands out elements one at a time, until it has none.
protocol IteratorProtocol {
    associatedtype Element
    mutating func next() -> Element?
}

// Something whose elements can be gone through in order. What it hands
// out is its Element; a type that is its own iterator names it by its
// next().
//
// makeIterator() makes the iterator; a type that is its own iterator --
// declares next() -- has one made for it, as Swift's Sequence gives it
// one, and its Element is what next() answers.
protocol Sequence {
    associatedtype Element
    associatedtype Iterator: IteratorProtocol
    func makeIterator() -> Iterator
}

// A type whose values can be told equal or not. A struct or enum that
// says it is one and writes no `==` of its own has one made for it, from
// its stored properties or its cases; `!=` comes with `==`.
protocol Equatable {
    static func == (lhs: Self, rhs: Self) -> Bool
}

// A type whose values are in an order. `>`, `<=` and `>=` come with `<`.
protocol Comparable: Equatable {
    static func < (lhs: Self, rhs: Self) -> Bool
}

// What a hash is made from: SipHash-1-3's state under the process's seed,
// as Swift's Hasher is. A value is fed in with combine and the hash comes
// out of finalize. Each of the three is the runtime's; see
// stdlib/runtime/collections.cpp.
struct Hasher {
    var v0: UInt64
    var v1: UInt64
    var v2: UInt64
    var v3: UInt64
    var tail: UInt64
    var length: UInt64

    init()
    mutating func combine<H: Hashable>(_ value: H)
    func finalize() -> Int
}

// A type whose values can be hashed, to be a Set's element or a
// Dictionary's key. Values that are == hash alike. A struct that says it is
// Hashable and writes no hash(into:) has one made for it, from its stored
// properties; an enum of cases alone, from its case.
protocol Hashable: Equatable {
    func hash(into hasher: inout Hasher)
}

// UTF-8, as the encoding String(decoding:as:) is told to read. Swift's
// Unicode.UTF8; nothing is made of it but its type.
enum UTF8 {}

// A type that says what it is as text: what interpolation and print show.
protocol CustomStringConvertible {
    var description: String { get }
}

// A value or the error that stood in for it.
enum Result<Success, Failure: Error> {
    case success(Success)
    case failure(Failure)
}

// A view of bytes somewhere in memory: where they start, if anywhere,
// and how many there are.
struct UnsafeRawBufferPointer {
    let baseAddress: UnsafeRawPointer?
    let count: Int
}

// Some of an array's elements, in place: those from startIndex up to
// endIndex, which count from the start of the array they are in.
struct ArraySlice<Element> {
    let base: [Element]
    let startIndex: Int
    let endIndex: Int
}

// Elements somewhere in memory, to read: where they start, if anywhere,
// and how many there are.
struct UnsafeBufferPointer<Element> {
    let baseAddress: UnsafePointer<Element>?
    let count: Int
}

// Elements somewhere in memory that may be written: where they start, if
// anywhere, and how many there are.
struct UnsafeMutableBufferPointer<Element> {
    let baseAddress: UnsafeMutablePointer<Element>?
    let count: Int
}

// Bytes somewhere in memory that may be written: where they start, if
// anywhere, and how many there are.
struct UnsafeMutableRawBufferPointer {
    let baseAddress: UnsafeMutableRawPointer?
    let count: Int
}
