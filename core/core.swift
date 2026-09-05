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
