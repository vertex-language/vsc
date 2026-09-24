// A literal operand has no type of its own to match a declaration
// with, and an operator's operands have no context until the operator
// is known -- which is a circle. Both literals defaulted to Int, so
// an operator declared over Int32 did not fit, and the result fell
// back to the left operand's type: wrong whatever the operator
// returns, and silently so where the two happen to agree.
infix operator <+> : AdditionPrecedence
func <+> (a: Int32, b: Int32) -> Int32 { return a * 10 + b }

infix operator <=> : ComparisonPrecedence
func <=> (a: Int32, b: Int32) -> Bool { return a < b }

func sum() -> Int32 { return 2 <+> 3 }
func less() -> Bool { return 1 <=> 2 }
