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

// The size, stride and alignment of T, read as MemoryLayout<T>.size: the
// lowering answers them once T is known, so the enum has no members.
enum MemoryLayout<T> {}

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

// A type that says what it is as text for debugging: what print shows of
// it inside a collection, and debugPrint.
protocol CustomDebugStringConvertible {
    var debugDescription: String { get }
}

// Types that may be written as a literal. Each asks for an initializer,
// init(integerLiteral:), init(stringLiteral:) and so on, taking the
// literal as one of the core's types; the compiler calls it where such a
// literal is written for the type. Swift says what the literal is taken
// as with an associated type; here it is whatever the initializer takes.
// A type whose values can all be listed: an enum of cases alone that
// says so has allCases made for it, its cases in the order declared.
protocol CaseIterable {
    static var allCases: [Self] { get }
}

protocol ExpressibleByIntegerLiteral {}
protocol ExpressibleByFloatLiteral {}
protocol ExpressibleByBooleanLiteral {}
protocol ExpressibleByNilLiteral {}
protocol ExpressibleByUnicodeScalarLiteral {}
protocol ExpressibleByExtendedGraphemeClusterLiteral: ExpressibleByUnicodeScalarLiteral {}
protocol ExpressibleByStringLiteral: ExpressibleByExtendedGraphemeClusterLiteral {}
protocol ExpressibleByArrayLiteral {}
protocol ExpressibleByDictionaryLiteral {}

// One extended grapheme cluster: what a String is a collection of, and
// what a string literal of one is where a Character is wanted. It is the
// String of that one cluster, as Swift's Character is; algorithms.swift
// gives it its members.
struct Character {
    let _string: String
}

// A path from a Root to one of its values -- `\Person.address.city` --
// which reads the value, and writes it where every step of the path can
// be written. It is the pair of functions the path is.
struct KeyPath<Root, Value> {
    let _get: (Root) -> Value
    let _set: ((inout Root, Value) -> Void)?
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

// The values from lowerBound up to, but not including, upperBound: what
// `a..<b` makes. algorithms.swift gives it its members.
struct Range<Bound: Comparable> {
    let lowerBound: Bound
    let upperBound: Bound
}

// The values from lowerBound up to and including upperBound: what
// `a...b` makes.
struct ClosedRange<Bound: Comparable> {
    let lowerBound: Bound
    let upperBound: Bound
}

// The values short of upperBound: what `..<b` makes.
struct PartialRangeUpTo<Bound: Comparable> {
    let upperBound: Bound
}

// The values up to and including upperBound: what `...b` makes.
struct PartialRangeThrough<Bound: Comparable> {
    let upperBound: Bound
}

// The values from lowerBound on: what `a...` makes.
struct PartialRangeFrom<Bound: Comparable> {
    let lowerBound: Bound
}

// What for-in over a range of integers walks, when it is walked as a
// Sequence: Array(1...10), a range handed to something generic.
struct _IntRangeIterator {
    var _at: Int
    let _end: Int
}

// What for-in over an ArraySlice walks: its base's elements from _at up
// to _end.
struct _ArraySliceIterator<Element> {
    let _base: [Element]
    var _at: Int
    let _end: Int
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

// ---- Instructions ----

// Functions that are one machine instruction each, named by @_builtin as
// SIL names it (the operand's type is added: int_ctpop_Int64). The
// algorithms build Int's and Double's members out of these, as Swift's
// standard library builds them out of Builtin.int_ctpop_Int64.

@_builtin("int_ctpop")
func _popcount(_ x: Int) -> Int
@_builtin("int_ctlz")
func _leadingZeros(_ x: Int) -> Int
@_builtin("int_cttz")
func _trailingZeros(_ x: Int) -> Int
@_builtin("int_bswap")
func _byteSwapped(_ x: Int) -> Int
@_builtin("int_ctpop")
func _popcount(_ x: Int8) -> Int8
@_builtin("int_ctlz")
func _leadingZeros(_ x: Int8) -> Int8
@_builtin("int_cttz")
func _trailingZeros(_ x: Int8) -> Int8
@_builtin("int_bswap")
func _byteSwapped(_ x: Int8) -> Int8
@_builtin("int_ctpop")
func _popcount(_ x: Int16) -> Int16
@_builtin("int_ctlz")
func _leadingZeros(_ x: Int16) -> Int16
@_builtin("int_cttz")
func _trailingZeros(_ x: Int16) -> Int16
@_builtin("int_bswap")
func _byteSwapped(_ x: Int16) -> Int16
@_builtin("int_ctpop")
func _popcount(_ x: Int32) -> Int32
@_builtin("int_ctlz")
func _leadingZeros(_ x: Int32) -> Int32
@_builtin("int_cttz")
func _trailingZeros(_ x: Int32) -> Int32
@_builtin("int_bswap")
func _byteSwapped(_ x: Int32) -> Int32
@_builtin("int_ctpop")
func _popcount(_ x: Int64) -> Int64
@_builtin("int_ctlz")
func _leadingZeros(_ x: Int64) -> Int64
@_builtin("int_cttz")
func _trailingZeros(_ x: Int64) -> Int64
@_builtin("int_bswap")
func _byteSwapped(_ x: Int64) -> Int64
@_builtin("int_ctpop")
func _popcount(_ x: UInt) -> UInt
@_builtin("int_ctlz")
func _leadingZeros(_ x: UInt) -> UInt
@_builtin("int_cttz")
func _trailingZeros(_ x: UInt) -> UInt
@_builtin("int_bswap")
func _byteSwapped(_ x: UInt) -> UInt
@_builtin("int_ctpop")
func _popcount(_ x: UInt8) -> UInt8
@_builtin("int_ctlz")
func _leadingZeros(_ x: UInt8) -> UInt8
@_builtin("int_cttz")
func _trailingZeros(_ x: UInt8) -> UInt8
@_builtin("int_bswap")
func _byteSwapped(_ x: UInt8) -> UInt8
@_builtin("int_ctpop")
func _popcount(_ x: UInt16) -> UInt16
@_builtin("int_ctlz")
func _leadingZeros(_ x: UInt16) -> UInt16
@_builtin("int_cttz")
func _trailingZeros(_ x: UInt16) -> UInt16
@_builtin("int_bswap")
func _byteSwapped(_ x: UInt16) -> UInt16
@_builtin("int_ctpop")
func _popcount(_ x: UInt32) -> UInt32
@_builtin("int_ctlz")
func _leadingZeros(_ x: UInt32) -> UInt32
@_builtin("int_cttz")
func _trailingZeros(_ x: UInt32) -> UInt32
@_builtin("int_bswap")
func _byteSwapped(_ x: UInt32) -> UInt32
@_builtin("int_ctpop")
func _popcount(_ x: UInt64) -> UInt64
@_builtin("int_ctlz")
func _leadingZeros(_ x: UInt64) -> UInt64
@_builtin("int_cttz")
func _trailingZeros(_ x: UInt64) -> UInt64
@_builtin("int_bswap")
func _byteSwapped(_ x: UInt64) -> UInt64
@_builtin("int_sqrt")
func _sqrt(_ x: Double) -> Double
@_builtin("int_floor")
func _floor(_ x: Double) -> Double
@_builtin("int_ceil")
func _ceil(_ x: Double) -> Double
@_builtin("int_trunc")
func _trunc(_ x: Double) -> Double
@_builtin("int_rint")
func _rint(_ x: Double) -> Double
@_builtin("int_fabs")
func _fabs(_ x: Double) -> Double
@_builtin("int_copysign")
func _copysign(_ x: Double, _ y: Double) -> Double
@_builtin("int_sqrt")
func _sqrt(_ x: Float) -> Float
@_builtin("int_floor")
func _floor(_ x: Float) -> Float
@_builtin("int_ceil")
func _ceil(_ x: Float) -> Float
@_builtin("int_trunc")
func _trunc(_ x: Float) -> Float
@_builtin("int_rint")
func _rint(_ x: Float) -> Float
@_builtin("int_fabs")
func _fabs(_ x: Float) -> Float
@_builtin("int_copysign")
func _copysign(_ x: Float, _ y: Float) -> Float
@_builtin("bitcast")
func _bits(_ x: Double) -> UInt64
@_builtin("bitcast")
func _double(_ bits: UInt64) -> Double
@_builtin("bitcast")
func _bits(_ x: Float) -> UInt32
@_builtin("bitcast")
func _float(_ bits: UInt32) -> Float
prefix func - (operand: Int8) -> Int8
prefix func - (operand: Int16) -> Int16
@_builtin("sadd_with_overflow")
func _addingReportingOverflow(_ a: Int, _ b: Int) -> (Int, Bool)
@_builtin("ssub_with_overflow")
func _subtractingReportingOverflow(_ a: Int, _ b: Int) -> (Int, Bool)
@_builtin("smul_with_overflow")
func _multipliedReportingOverflow(_ a: Int, _ b: Int) -> (Int, Bool)
@_builtin("sadd_with_overflow")
func _addingReportingOverflow(_ a: Int8, _ b: Int8) -> (Int8, Bool)
@_builtin("ssub_with_overflow")
func _subtractingReportingOverflow(_ a: Int8, _ b: Int8) -> (Int8, Bool)
@_builtin("smul_with_overflow")
func _multipliedReportingOverflow(_ a: Int8, _ b: Int8) -> (Int8, Bool)
@_builtin("sadd_with_overflow")
func _addingReportingOverflow(_ a: Int16, _ b: Int16) -> (Int16, Bool)
@_builtin("ssub_with_overflow")
func _subtractingReportingOverflow(_ a: Int16, _ b: Int16) -> (Int16, Bool)
@_builtin("smul_with_overflow")
func _multipliedReportingOverflow(_ a: Int16, _ b: Int16) -> (Int16, Bool)
@_builtin("sadd_with_overflow")
func _addingReportingOverflow(_ a: Int32, _ b: Int32) -> (Int32, Bool)
@_builtin("ssub_with_overflow")
func _subtractingReportingOverflow(_ a: Int32, _ b: Int32) -> (Int32, Bool)
@_builtin("smul_with_overflow")
func _multipliedReportingOverflow(_ a: Int32, _ b: Int32) -> (Int32, Bool)
@_builtin("sadd_with_overflow")
func _addingReportingOverflow(_ a: Int64, _ b: Int64) -> (Int64, Bool)
@_builtin("ssub_with_overflow")
func _subtractingReportingOverflow(_ a: Int64, _ b: Int64) -> (Int64, Bool)
@_builtin("smul_with_overflow")
func _multipliedReportingOverflow(_ a: Int64, _ b: Int64) -> (Int64, Bool)
@_builtin("uadd_with_overflow")
func _addingReportingOverflow(_ a: UInt, _ b: UInt) -> (UInt, Bool)
@_builtin("usub_with_overflow")
func _subtractingReportingOverflow(_ a: UInt, _ b: UInt) -> (UInt, Bool)
@_builtin("umul_with_overflow")
func _multipliedReportingOverflow(_ a: UInt, _ b: UInt) -> (UInt, Bool)
@_builtin("uadd_with_overflow")
func _addingReportingOverflow(_ a: UInt8, _ b: UInt8) -> (UInt8, Bool)
@_builtin("usub_with_overflow")
func _subtractingReportingOverflow(_ a: UInt8, _ b: UInt8) -> (UInt8, Bool)
@_builtin("umul_with_overflow")
func _multipliedReportingOverflow(_ a: UInt8, _ b: UInt8) -> (UInt8, Bool)
@_builtin("uadd_with_overflow")
func _addingReportingOverflow(_ a: UInt16, _ b: UInt16) -> (UInt16, Bool)
@_builtin("usub_with_overflow")
func _subtractingReportingOverflow(_ a: UInt16, _ b: UInt16) -> (UInt16, Bool)
@_builtin("umul_with_overflow")
func _multipliedReportingOverflow(_ a: UInt16, _ b: UInt16) -> (UInt16, Bool)
@_builtin("uadd_with_overflow")
func _addingReportingOverflow(_ a: UInt32, _ b: UInt32) -> (UInt32, Bool)
@_builtin("usub_with_overflow")
func _subtractingReportingOverflow(_ a: UInt32, _ b: UInt32) -> (UInt32, Bool)
@_builtin("umul_with_overflow")
func _multipliedReportingOverflow(_ a: UInt32, _ b: UInt32) -> (UInt32, Bool)
@_builtin("uadd_with_overflow")
func _addingReportingOverflow(_ a: UInt64, _ b: UInt64) -> (UInt64, Bool)
@_builtin("usub_with_overflow")
func _subtractingReportingOverflow(_ a: UInt64, _ b: UInt64) -> (UInt64, Bool)
@_builtin("umul_with_overflow")
func _multipliedReportingOverflow(_ a: UInt64, _ b: UInt64) -> (UInt64, Bool)
@_builtin("int_smulhi")
func _multipliedHigh(_ a: Int, _ b: Int) -> Int
@_builtin("int_smulhi")
func _multipliedHigh(_ a: Int64, _ b: Int64) -> Int64
@_builtin("int_umulhi")
func _multipliedHigh(_ a: UInt, _ b: UInt) -> UInt
@_builtin("int_umulhi")
func _multipliedHigh(_ a: UInt64, _ b: UInt64) -> UInt64
@_builtin("cmp_eq")
func _same(_ a: Any.Type, _ b: Any.Type) -> Bool
@_silgen_name("vertex_fatal_error")
func _fatalErrorMessage(_ message: String) -> Never

// Whether object is the only strong reference to its instance: what copy
// on write asks before it writes. The runtime reads the count in place,
// and ignores the metadata a generic call hands it after the reference.
// Swift constrains T to AnyObject; a conformance to that carries a table
// this compiler does not name yet, so T is left unconstrained.
@_silgen_name("vertex_is_uniquely_referenced")
func isKnownUniquelyReferenced<T>(_ object: inout T) -> Bool

// ---- String, as Characters ----

// Where the Character at byte offset at of s ends, and where the one that
// ends at at starts: the extended grapheme cluster boundaries the runtime
// finds. algorithms.swift walks a String by Characters with them.
@_silgen_name("vertex_string_character_end")
func _characterEnd(_ s: String, _ at: Int) -> Int
@_silgen_name("vertex_string_character_start")
func _characterStart(_ s: String, _ at: Int) -> Int
// The String of s's bytes from up to to.
@_silgen_name("vertex_string_slice")
func _stringSlice(_ s: String, _ from: Int, _ to: Int) -> String
// How many bytes s's UTF-8 is.
@_silgen_name("vertex_string_utf8_count")
func _utf8Count(_ s: String) -> Int
// s with each scalar's full case mapping in its place.
@_silgen_name("vertex_string_uppercased")
func _uppercased(_ s: String) -> String
@_silgen_name("vertex_string_lowercased")
func _lowercased(_ s: String) -> String

// The scalar at byte offset at of s, in the low 21 bits, and the offset
// after it, above them.
@_silgen_name("vertex_string_scalar")
func _scalarAt(_ s: String, _ at: Int) -> Int
// The String of one scalar.
@_silgen_name("vertex_scalar_string")
func _scalarString(_ value: UInt32) -> String
// A scalar's properties: flags, general category and numeric type, a
// byte each; and its value where it is a whole number, or -1.
@_silgen_name("vertex_scalar_properties")
func _scalarProperties(_ value: UInt32) -> UInt32
@_silgen_name("vertex_scalar_whole_number")
func _scalarWholeNumber(_ value: UInt32) -> Int

// Swift's namespace for Unicode's types: `Unicode.Scalar`.
enum Unicode {}

// A Unicode scalar value: what a Character is made of. `Unicode.Scalar`
// names it too.
struct UnicodeScalar {
    let value: UInt32
}

// A String's scalars, and its UTF-16 code units: its unicodeScalars and
// utf16. `String.UnicodeScalarView` and `String.UTF16View` name them.
struct _UnicodeScalarView {
    let _string: String
}

struct _UTF16View {
    let _string: String
}

// What for-in over a String's unicodeScalars walks, from byte _at on.
struct _UnicodeScalarIterator {
    let _string: String
    var _at: Int
}

// A position in a String: the offset of a Character's first byte in the
// String's UTF-8. `String.Index` names it.
struct _StringIndex {
    let _offset: Int
}

// Some of a String's Characters, in place: those from byte _start up to
// _end of the String they are in, whose indices are theirs.
struct Substring {
    let _base: String
    let _start: Int
    let _end: Int
}

// What for-in over a String walks: its Characters, in order, from byte
// _at up to _end.
struct _StringIterator {
    let _string: String
    var _at: Int
    let _end: Int
}
