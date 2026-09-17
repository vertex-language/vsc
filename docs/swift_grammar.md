# The Swift Grammar

A grammar for the Swift programming language, expressed using the notation of the Java Language Specification (JLS) — a BNF variant chosen for being shorter and easier to read than other common styles. 

This document describes a Swift translation unit as it appears in Swift 6. The grammar productions are sourced from the formal summary of *The Swift Programming Language* specification.

## 1. Grammar Notation

The definition of a nonterminal is introduced by its name, followed by a colon. One or more alternative definitions for the nonterminal then follow on succeeding lines.

```text
IfStatement:
if ConditionList CodeBlock [ElseClause]

```

* **CamelCase** words are nonterminals. Everything else — lowercase keywords, punctuation, operators — is a terminal, to appear exactly as written.
* `{x}` denotes zero or more occurrences of `x`.
* `[x]` denotes zero or one occurrence of `x` (i.e., `x` is optional).
* `(one of)` on the line following the colon signifies that each of the symbols on the succeeding line(s) is a separate alternative definition.
* `but not` indicates expansions that are excluded.
* A narrative phrase in parenthesis describes character sets where enumeration is impractical.
* A terminal brace or bracket is written in single quotes to distinguish it from the meta-syntax: `'{'`, `'}'`, `'['`, `']'`. The single quote itself is not a terminal.

## 2. Lexical Structure

### 2.1 Goal Symbol

```text
TopLevelDeclaration:
{Statement}

```

### 2.2 Identifiers

```text
Identifier:
IdentifierHead {IdentifierCharacter}
`RawIdentifier`
ImplicitParameterName
PropertyWrapperProjection

RawIdentifier:
Any characters but a backtick, a backslash, a line break, or
whitespace other than the space itself; not empty, not all spaces,
and not made only of OperatorCharacters.

IdentifierHead:
(one of)
a b c d e f g h i j k l m n o p q r s t u v w x y z
A B C D E F G H I J K L M N O P Q R S T U V W X Y Z
_
U+00A8, U+00AA, U+00AD, U+00AF, U+00B2–U+00B5, U+00B7–U+00BA
U+00BC–U+00BE, U+00C0–U+00D6, U+00D8–U+00F6, U+00F8–U+00FF
U+0100–U+02FF, U+0370–U+167F, U+1681–U+180D, U+180F–U+1DBF
U+1E00–U+1FFF, U+200B–U+200D, U+202A–U+202E, U+203F–U+2040
U+2054, U+2060–U+20CF, U+2100–U+218F, U+2460–U+24FF
U+2776–U+2793, U+2C00–U+2DFF, U+2E80–U+2FFF, U+3004–U+3007
U+3021–U+302F, U+3031–U+303F, U+3040–U+D7FF, U+F900–U+FD3D
U+FD40–U+FDCF, U+FDF0–U+FE1F, U+FE30–U+FE44, U+FE47–U+FFF8
U+10000–U+1FFFD, U+20000–U+2FFFD, U+30000–U+3FFFD
U+40000–U+4FFFD, U+50000–U+5FFFD, U+60000–U+6FFFD
U+70000–U+7FFFD, U+80000–U+8FFFD, U+90000–U+9FFFD
U+A0000–U+AFFFD, U+B0000–U+BFFFD, U+C0000–U+CFFFD
U+D0000–U+DFFFD, U+E0000–U+EFFFD

(These are ranges, not Unicode categories. An emoji is an
IdentifierHead — U+1F600 is in U+10000–U+1FFFD — and U+00A9 © is not,
though both are symbols.)

IdentifierCharacter:
Digit
IdentifierHead
U+0300–U+036F, U+1DC0–U+1DFF, U+20D0–U+20FF, U+FE20–U+FE2F

IdentifierCharacters:
IdentifierCharacter {IdentifierCharacter}

ImplicitParameterName:
$ {DecimalDigit}

PropertyWrapperProjection:
$ IdentifierCharacters

```

### 2.3 Literals

```text
Literal:
NumericLiteral
StringLiteral
BooleanLiteral
NilLiteral
RegexLiteral

```

#### 2.3.1 Numeric Literals

```text
NumericLiteral:
- IntegerLiteral
IntegerLiteral
- FloatingPointLiteral
FloatingPointLiteral

IntegerLiteral:
BinaryLiteral
OctalLiteral
DecimalLiteral
HexadecimalLiteral

BinaryLiteral:
0 b BinaryDigit {BinaryLiteralCharacter}

BinaryDigit: (one of)
0 1

BinaryLiteralCharacter:
BinaryDigit
_

OctalLiteral:
0 o OctalDigit {OctalLiteralCharacter}

OctalDigit: (one of)
0 1 2 3 4 5 6 7

OctalLiteralCharacter:
OctalDigit
_

DecimalLiteral:
DecimalDigit {DecimalLiteralCharacter}

DecimalDigit: (one of)
0 1 2 3 4 5 6 7 8 9

DecimalLiteralCharacter:
DecimalDigit
_

HexadecimalLiteral:
0 x HexadecimalDigit {HexadecimalLiteralCharacter}

HexadecimalDigit: (one of)
0 1 2 3 4 5 6 7 8 9 a b c d e f A B C D E F

HexadecimalLiteralCharacter:
HexadecimalDigit
_

FloatingPointLiteral:
DecimalLiteral [DecimalFraction] [DecimalExponent]
HexadecimalLiteral [HexadecimalFraction] HexadecimalExponent

DecimalFraction:
. DecimalLiteral

DecimalExponent:
FloatingPointE [Sign] DecimalLiteral

HexadecimalFraction:
. HexadecimalDigit {HexadecimalLiteralCharacter}

HexadecimalExponent:
FloatingPointP [Sign] DecimalLiteral

FloatingPointE: (one of)
e E

FloatingPointP: (one of)
p P

Sign: (one of)
+ -

```

#### 2.3.2 String Literals

```text
StringLiteral:
StaticStringLiteral
InterpolatedStringLiteral

StringLiteralOpeningDelimiter:
{ExtendedStringLiteralDelimiter} "

StringLiteralClosingDelimiter:
" {ExtendedStringLiteralDelimiter}

StaticStringLiteral:
StringLiteralOpeningDelimiter {QuotedTextItem} StringLiteralClosingDelimiter
MultilineStringLiteralOpeningDelimiter {MultilineQuotedTextItem} MultilineStringLiteralClosingDelimiter

ExtendedStringLiteralDelimiter:
# {#}

QuotedTextItem:
EscapedCharacter
(any Unicode scalar value except ", \, U+000A, or U+000D)

MultilineStringLiteralOpeningDelimiter:
{ExtendedStringLiteralDelimiter} """

MultilineStringLiteralClosingDelimiter:
""" {ExtendedStringLiteralDelimiter}

MultilineQuotedTextItem:
EscapedCharacter
(any Unicode scalar value except \ or the multiline closing delimiter)
EscapedNewline

InterpolatedStringLiteral:
StringLiteralOpeningDelimiter {InterpolatedTextItem} StringLiteralClosingDelimiter
MultilineStringLiteralOpeningDelimiter {MultilineInterpolatedTextItem} MultilineStringLiteralClosingDelimiter

InterpolatedTextItem:
QuotedTextItem
\ {ExtendedStringLiteralDelimiter} ( Expression )

MultilineInterpolatedTextItem:
MultilineQuotedTextItem
\ {ExtendedStringLiteralDelimiter} ( Expression )

EscapeSequence:
\ {ExtendedStringLiteralDelimiter}

EscapedCharacter:
EscapeSequence (one of) 0 \ t n r " '
EscapeSequence u '{' HexadecimalDigit {HexadecimalDigit} '}'

EscapedNewline:
EscapeSequence LineTerminator

LineTerminator:
(the ASCII LF character)
(the ASCII CR character)
(the ASCII CR character followed by the ASCII LF character)

```

*Note: The number of `#` characters in a closing delimiter must exactly match the number in its corresponding opening delimiter.*

#### 2.3.3 Boolean, Nil, and Regex Literals

```text
BooleanLiteral: (one of)
true false

NilLiteral:
nil

RegexLiteral:
RegexLiteralOpeningDelimiter RegexLiteralPattern RegexLiteralClosingDelimiter

RegexLiteralOpeningDelimiter:
{ExtendedRegexLiteralDelimiter} /

RegexLiteralClosingDelimiter:
/ {ExtendedRegexLiteralDelimiter}

RegexLiteralPattern:
(any regular expression pattern)

ExtendedRegexLiteralDelimiter:
# {#}

```

### 2.4 Operators

```text
Operator:
OperatorHead {OperatorCharacter}
. OperatorCharacter {OperatorCharacter}

OperatorHead: (one of)
/ = - + ! * % < > & | ^ ~ ?
(any Unicode scalar value reserved for operators)

OperatorCharacter:
OperatorHead
(any Unicode scalar value reserved for operator combining characters)

```

---

## 3. Types

```text
Type:
FunctionType
ArrayType
DictionaryType
TupleType
OptionalType
ImplicitlyUnwrappedOptionalType
MetatypeType
AnyType
SelfType
OpaqueType
BoxedProtocolType
ProtocolCompositionType
TypeIdentifier
( Type )
PackExpansionType
PackReferenceType
MacroExpansionType

```

### 3.1 Type Identifiers and Composition

```text
TypeIdentifier:
TypeName [GenericArgumentClause]
TypeName [GenericArgumentClause] . TypeIdentifier

TypeName:
Identifier

ProtocolCompositionType:
TypeIdentifier {& TypeIdentifier}

AnyType:
Any

SelfType:
Self

OpaqueType:
[AttributeList] some Type

BoxedProtocolType:
[AttributeList] any Type

```

### 3.2 Function and Tuple Types

```text
FunctionType:
[AttributeList] FunctionTypeParameters [async] [ThrowsClause] -> Type
[AttributeList] FunctionTypeParameters [async] rethrows -> Type

FunctionTypeParameters:
( [ParameterList] )

ParameterList:
Parameter {, Parameter}

Parameter:
[ArgumentLabel :] [ParameterModifierList] Type [...]

ArgumentLabel:
Identifier
_

ParameterModifierList:
ParameterModifier {ParameterModifier}

ParameterModifier: (one of)
inout  borrowing  consuming

ThrowsClause:
throws
throws ( Type )

TupleType:
( [TupleTypeElementList] )

TupleTypeElementList:
TupleTypeElement {, TupleTypeElement}

TupleTypeElement:
[Identifier :] Type

```

### 3.3 Container and Metatypes

```text
ArrayType:
'[' Type ']'

DictionaryType:
'[' Type : Type ']'

OptionalType:
Type ?

ImplicitlyUnwrappedOptionalType:
Type !

MetatypeType:
Type . TypeKeyword
Type . ProtocolKeyword

TypeKeyword:
Type

ProtocolKeyword:
Protocol

```

### 3.4 Packs and Macros

```text
PackExpansionType:
repeat Type

PackReferenceType:
each Type

MacroExpansionType:
# Identifier [GenericArgumentClause] [FunctionCallArgumentClause] [TrailingClosures]

```

---

## 4. Expressions

```text
Expression:
TryOperator [AwaitOperator] PrefixExpression [BinaryExpressions]
AwaitOperator PrefixExpression [BinaryExpressions]
PrefixExpression [BinaryExpressions]
ClosureExpression
PackExpansionExpression

```

### 4.1 Operators and Prefix Expressions

```text
TryOperator:
try
try ?
try !

AwaitOperator:
await

PrefixExpression:
PrefixOperator PostfixExpression
InOutExpression
consume Expression
copy Expression
borrow Expression
PostfixExpression

InOutExpression:
& Expression

```

### 4.2 Binary Expressions

```text
BinaryExpressions:
BinaryExpression {BinaryExpression}

BinaryExpression:
BinaryOperator PrefixExpression
AssignmentOperator PrefixExpression
ConditionalOperator PrefixExpression
TypeCastingOperator

AssignmentOperator:
=

ConditionalOperator:
? Expression :

TypeCastingOperator:
is Type
as Type
as? Type
as! Type

```

### 4.3 Postfix Expressions

```text
PostfixExpression:
PrimaryExpression
PostfixExpression PostfixOperator
PostfixExpression FunctionCallArgumentClause [TrailingClosures]
PostfixExpression TrailingClosures
PostfixExpression InitializerArgumentClause
PostfixExpression ExplicitMemberExpression
PostfixExpression PostfixSelfExpression
PostfixExpression SubscriptArgumentClause
PostfixExpression ForcedValueExpression
PostfixExpression OptionalChainingExpression

```

### 4.4 Primary Expressions

```text
PrimaryExpression:
Identifier [GenericArgumentClause]
LiteralExpression
SelfExpression
SuperclassExpression
ClosureExpression
ParenthesizedExpression
TupleExpression
ImplicitMemberExpression
WildcardExpression
KeyPathExpression
SelectorExpression
KeyPathStringExpression
MacroExpansionExpression

```

### 4.5 Literal Expressions

```text
LiteralExpression:
Literal
ArrayLiteral
DictionaryLiteral
PlaygroundLiteral
#file
#fileID
#filePath
#line
#column
#function
#dsohandle

ArrayLiteral:
'[' [ArrayLiteralItems] ']'

ArrayLiteralItems:
ArrayLiteralItem {, ArrayLiteralItem} [,]

ArrayLiteralItem:
Expression

DictionaryLiteral:
'[' DictionaryLiteralItems ']'
'[' : ']'

DictionaryLiteralItems:
DictionaryLiteralItem {, DictionaryLiteralItem} [,]

DictionaryLiteralItem:
Expression : Expression

PlaygroundLiteral:
#colorLiteral ( red : Expression , green : Expression , blue : Expression , alpha : Expression )
#fileLiteral ( resourceName : Expression )
#imageLiteral ( resourceName : Expression )

```

### 4.6 Closures

```text
ClosureExpression:
'{' [AttributeList] [ClosureSignature] [Statements] '}'

ClosureSignature:
[CaptureList] [ClosureParameterClause] [async] [ThrowsClause] [FunctionResult] in
[CaptureList] [ClosureParameterClause] [async] rethrows [FunctionResult] in
CaptureList in

CaptureList:
'[' CaptureListItems ']'

CaptureListItems:
CaptureListItem {, CaptureListItem}

CaptureListItem:
[CaptureSpecifier] Expression

CaptureSpecifier: (one of)
weak  unowned  unowned(safe)  unowned(unsafe)

ClosureParameterClause:
( )
( ClosureParameterList )
IdentifierList

ClosureParameterList:
ClosureParameter {, ClosureParameter}

ClosureParameter:
[ClosureParameterName] [TypeAnnotation]

ClosureParameterName:
Identifier

FunctionResult:
-> Type

TrailingClosures:
TrailingClosure {LabeledTrailingClosure}

TrailingClosure:
ClosureExpression

LabeledTrailingClosure:
Identifier : ClosureExpression

```

### 4.7 Function Calls and Member Access

```text
FunctionCallArgumentClause:
( )
( FunctionCallArgumentList )

FunctionCallArgumentList:
FunctionCallArgument {, FunctionCallArgument}

FunctionCallArgument:
Expression
Identifier : Expression
Operator
Identifier : Operator

InitializerArgumentClause:
. init [FunctionCallArgumentClause]

ExplicitMemberExpression:
. Identifier [GenericArgumentClause]
. Identifier ( ArgumentNames )

ArgumentNames:
ArgumentName {ArgumentName}

ArgumentName:
Identifier :

ImplicitMemberExpression:
. Identifier

PostfixSelfExpression:
. self

SubscriptArgumentClause:
'[' FunctionCallArgumentList ']'

ForcedValueExpression:
!

OptionalChainingExpression:
?

```

### 4.8 Key-Paths and Selectors

```text
KeyPathExpression:
\ [Type] [.] KeyPathComponents
\ [Type] SubscriptArgumentClause [KeyPathComponents]

KeyPathComponents:
KeyPathComponent {. KeyPathComponent}

KeyPathComponent:
Identifier [FunctionCallArgumentClause]
'[' FunctionCallArgumentList ']'
?
!
self

SelectorExpression:
#selector ( Expression )
#selector ( getter : Expression )
#selector ( setter : Expression )

KeyPathStringExpression:
#keyPath ( Expression )

```

### 4.9 Self, Super, Wildcard, and Packs

```text
SelfExpression:
self
self . Identifier
self '[' [FunctionCallArgumentList] ']'
self . init

SuperclassExpression:
super . Identifier
super '[' [FunctionCallArgumentList] ']'
super . init

WildcardExpression:
_

ParenthesizedExpression:
( Expression )

TupleExpression:
( TupleElement {, TupleElement} )

TupleElement:
[Identifier :] Expression

PackExpansionExpression:
repeat Expression

MacroExpansionExpression:
# Identifier [GenericArgumentClause] [FunctionCallArgumentClause] [TrailingClosures]

```

---

## 5. Statements

```text
Statement:
Expression [;]
Declaration [;]
LoopStatement [;]
BranchStatement [;]
LabeledStatement [;]
ControlTransferStatement [;]
DeferStatement [;]
DoStatement [;]
CompilerControlStatement

Statements:
Statement {Statement}

CodeBlock:
'{' [Statements] '}'

```

### 5.1 Loops

```text
LoopStatement:
ForInStatement
WhileStatement
RepeatWhileStatement

ForInStatement:
for [await] [case] Pattern in Expression [WhereClause] CodeBlock

WhileStatement:
while ConditionList CodeBlock

RepeatWhileStatement:
repeat CodeBlock while Expression

```

### 5.2 Branches

```text
BranchStatement:
IfStatement
GuardStatement
SwitchStatement

IfStatement:
if ConditionList CodeBlock [ElseClause]

ElseClause:
else CodeBlock
else IfStatement

GuardStatement:
guard ConditionList else CodeBlock

SwitchStatement:
switch Expression '{' {SwitchCase} '}'

SwitchCase:
CaseLabel Statements
DefaultLabel Statements
ConditionalSwitchCase

CaseLabel:
[AttributeList] case CaseItemList [WhereClause] :

CaseItemList:
CaseItem {, CaseItem}

CaseItem:
Pattern [WhereClause]

DefaultLabel:
[AttributeList] default :

```

### 5.3 Conditions

```text
ConditionList:
Condition {, Condition}

Condition:
Expression
AvailabilityCondition
CaseCondition
OptionalBindingCondition

AvailabilityCondition:
#available ( AvailabilityArguments )
#unavailable ( AvailabilityArguments )

AvailabilityArguments:
AvailabilityArgument {, AvailabilityArgument}

AvailabilityArgument:
PlatformName PlatformVersion
*

PlatformName: (one of)
iOS iOSApplicationExtension
macOS macOSApplicationExtension
macCatalyst macCatalystApplicationExtension
watchOS watchOSApplicationExtension
tvOS tvOSApplicationExtension
visionOS visionOSApplicationExtension

PlatformVersion:
DecimalDigit {. DecimalDigit}

CaseCondition:
case Pattern Initializer

OptionalBindingCondition:
let Pattern [Initializer]
var Pattern [Initializer]

```

### 5.4 Labeled and Control Transfer Statements

```text
LabeledStatement:
StatementLabel LoopStatement
StatementLabel IfStatement
StatementLabel SwitchStatement
StatementLabel DoStatement

StatementLabel:
LabelName :

LabelName:
Identifier

ControlTransferStatement:
BreakStatement
ContinueStatement
FallthroughStatement
ReturnStatement
ThrowStatement
DiscardStatement

BreakStatement:
break [LabelName]

ContinueStatement:
continue [LabelName]

FallthroughStatement:
fallthrough

ReturnStatement:
return [Expression]

ThrowStatement:
throw Expression

DiscardStatement:
discard Expression

```

### 5.5 Defer and Do Statements

```text
DeferStatement:
defer CodeBlock

DoStatement:
do [ThrowsClause] CodeBlock {CatchClause}

CatchClause:
catch [CatchPatternList] CodeBlock

CatchPatternList:
CatchPattern {, CatchPattern}

CatchPattern:
Pattern [WhereClause]

```

### 5.6 Compiler Control Statements

```text
CompilerControlStatement:
ConditionalCompilationBlock
LineControlStatement
DiagnosticStatement

ConditionalCompilationBlock:
IfDirectiveClause {ElseIfDirectiveClause} [ElseDirectiveClause] #endif

IfDirectiveClause:
#if CompilationCondition {Statement}

ElseIfDirectiveClause:
#elseif CompilationCondition {Statement}

ElseDirectiveClause:
#else {Statement}

CompilationCondition:
PlatformCondition
Identifier
BooleanLiteral
( CompilationCondition )
! CompilationCondition
CompilationCondition && CompilationCondition
CompilationCondition || CompilationCondition

PlatformCondition:
os ( OperatingSystem )
arch ( Architecture )
swift ( >= SwiftVersion )
swift ( < SwiftVersion )
compiler ( >= CompilerVersion )
compiler ( < CompilerVersion )
canImport ( ImportPath )
targetEnvironment ( Environment )
hasAttribute ( AttributeName )

OperatingSystem: (one of)
macOS iOS watchOS tvOS visionOS Linux Windows

Architecture: (one of)
i386 x86_64 arm arm64

SwiftVersion:
DecimalDigit {. DecimalDigit}

CompilerVersion:
DecimalDigit {. DecimalDigit}

ImportPath:
Identifier {. Identifier}

Environment: (one of)
simulator macCatalyst

LineControlStatement:
#sourceLocation ( file : StringLiteral , line : IntegerLiteral )
#sourceLocation ( )

DiagnosticStatement:
#error ( StringLiteral )
#warning ( StringLiteral )

```

---

## 6. Declarations

```text
Declaration:
ImportDeclaration
ConstantDeclaration
VariableDeclaration
TypealiasDeclaration
FunctionDeclaration
EnumDeclaration
StructDeclaration
ClassDeclaration
ActorDeclaration
ProtocolDeclaration
InitializerDeclaration
DeinitializerDeclaration
ExtensionDeclaration
SubscriptDeclaration
OperatorDeclaration
PrecedenceGroupDeclaration
MacroDeclaration

```

### 6.1 Modifiers

```text
DeclarationModifiers:
DeclarationModifier {DeclarationModifier}

DeclarationModifier: (one of)
class  convenience  dynamic  final  infix  lazy  optional  override  postfix  prefix  required  static
unowned  unowned(safe)  unowned(unsafe)  weak
AccessLevelModifier
MutationModifier
ActorIsolationModifier

AccessLevelModifier:
(one of) private fileprivate internal package public open
(one of) private fileprivate internal package public open ( set )

MutationModifier: (one of)
mutating  nonmutating

ActorIsolationModifier: (one of)
nonisolated  isolated

```

*Note: `convenience` (for `convenience init`) and the legacy operator-function modifiers `prefix`, `postfix`, and `infix` (for declarations like `prefix func +`) are included here alongside the other declaration modifiers — these were missing from the earlier draft. `isolated` is listed alongside `nonisolated` for completeness, though it's most commonly seen as a parameter modifier (e.g. `isolated any Actor`) rather than on the declaration itself; treat its inclusion at this position as provisional if you need to match the official grammar appendix exactly.*

### 6.2 Imports, Constants, and Variables

```text
ImportDeclaration:
[AttributeList] import [ImportKind] ImportPath

ImportKind: (one of)
typealias struct class enum protocol let var func macro

ConstantDeclaration:
[AttributeList] [DeclarationModifiers] let PatternInitializerList

PatternInitializerList:
PatternInitializer {, PatternInitializer}

PatternInitializer:
Pattern [Initializer]

Initializer:
= Expression

VariableDeclaration:
[AttributeList] [DeclarationModifiers] var PatternInitializerList
[AttributeList] [DeclarationModifiers] var VariableName [TypeAnnotation] CodeBlock
[AttributeList] [DeclarationModifiers] var VariableName [TypeAnnotation] GetterSetterBlock
[AttributeList] [DeclarationModifiers] var VariableName [TypeAnnotation] GetterKeywordClause
[AttributeList] [DeclarationModifiers] var VariableName Initializer WillSetDidSetBlock
[AttributeList] [DeclarationModifiers] var VariableName [TypeAnnotation] Initializer WillSetDidSetBlock

VariableName:
Identifier

```

### 6.3 Getters, Setters, and Observers

```text
GetterSetterBlock:
'{' GetterClause [SetterClause] '}'
'{' SetterClause GetterClause '}'

GetterClause:
[AttributeList] [MutationModifier] get [async] [ThrowsClause] CodeBlock

SetterClause:
[AttributeList] [MutationModifier] set [( SetterName )] CodeBlock

SetterName:
Identifier

GetterKeywordClause:
[AttributeList] [MutationModifier] get [async] [ThrowsClause]

WillSetDidSetBlock:
'{' WillSetClause [DidSetClause] '}'
'{' DidSetClause [WillSetClause] '}'

WillSetClause:
[AttributeList] willSet [( SetterName )] CodeBlock

DidSetClause:
[AttributeList] didSet [( SetterName )] CodeBlock

```

### 6.4 Typealiases and Functions

```text
TypealiasDeclaration:
[AttributeList] [AccessLevelModifier] typealias TypealiasName [GenericParameterClause] = Type

TypealiasName:
Identifier

FunctionDeclaration:
[AttributeList] [DeclarationModifiers] func FunctionName [GenericParameterClause] FunctionSignature [GenericWhereClause] [FunctionBody]

FunctionName:
Identifier
Operator

FunctionSignature:
( [ParameterList] ) [async] [ThrowsClause] [FunctionResult]
( [ParameterList] ) [async] rethrows [FunctionResult]

FunctionBody:
CodeBlock

```

### 6.5 Enums, Structs, Classes, and Actors

```text
EnumDeclaration:
[AttributeList] [DeclarationModifiers] enum EnumName [GenericParameterClause] [TypeInheritanceClause] [GenericWhereClause] '{' {EnumMember} '}'

EnumName:
Identifier

EnumMember:
Declaration
CompilerControlStatement
EnumCaseDeclaration

EnumCaseDeclaration:
[AttributeList] [DeclarationModifiers] indirect case EnumCasePatternList
[AttributeList] [DeclarationModifiers] case EnumCasePatternList

EnumCasePatternList:
EnumCasePattern {, EnumCasePattern}

EnumCasePattern:
EnumCaseName [TuplePattern] [RawValueAssignment]

EnumCaseName:
Identifier

RawValueAssignment:
= RawValueLiteral

RawValueLiteral:
NumericLiteral
StaticStringLiteral
BooleanLiteral

StructDeclaration:
[AttributeList] [DeclarationModifiers] struct StructName [GenericParameterClause] [TypeInheritanceClause] [GenericWhereClause] '{' {Declaration} '}'

StructName:
Identifier

ClassDeclaration:
[AttributeList] [DeclarationModifiers] class ClassName [GenericParameterClause] [TypeInheritanceClause] [GenericWhereClause] '{' {Declaration} '}'

ClassName:
Identifier

ActorDeclaration:
[AttributeList] [DeclarationModifiers] actor ActorName [GenericParameterClause] [TypeInheritanceClause] [GenericWhereClause] '{' {Declaration} '}'

ActorName:
Identifier

```

*Note: `RawValueAssignment` was tightened from a bare `Literal` to `RawValueLiteral`. Enum raw values may only be numeric, static-string, or boolean literals — `nil` and regex literals are not valid raw values.*

### 6.6 Protocols

```text
ProtocolDeclaration:
[AttributeList] [AccessLevelModifier] protocol ProtocolName [TypeInheritanceClause] [GenericWhereClause] '{' {ProtocolMember} '}'

ProtocolName:
Identifier

ProtocolMember:
ProtocolPropertyDeclaration
ProtocolMethodDeclaration
ProtocolInitializerDeclaration
ProtocolSubscriptDeclaration
ProtocolAssociatedTypeDeclaration
TypealiasDeclaration
CompilerControlStatement

ProtocolPropertyDeclaration:
[AttributeList] [DeclarationModifiers] var VariableName TypeAnnotation GetterSetterKeywordBlock

GetterSetterKeywordBlock:
'{' GetterKeywordClause [SetterKeywordClause] '}'
'{' SetterKeywordClause GetterKeywordClause '}'

SetterKeywordClause:
[AttributeList] [MutationModifier] set

ProtocolMethodDeclaration:
[AttributeList] [DeclarationModifiers] func FunctionName [GenericParameterClause] FunctionSignature [GenericWhereClause]

ProtocolInitializerDeclaration:
[AttributeList] [DeclarationModifiers] init [?] [!] ( [ParameterList] ) [async] [ThrowsClause] [GenericWhereClause]
[AttributeList] [DeclarationModifiers] init [?] [!] ( [ParameterList] ) [async] rethrows [GenericWhereClause]

ProtocolSubscriptDeclaration:
[AttributeList] [DeclarationModifiers] subscript ( [ParameterList] ) -> Type [GenericWhereClause] GetterSetterKeywordBlock

ProtocolAssociatedTypeDeclaration:
[AttributeList] [AccessLevelModifier] associatedtype TypealiasName [TypeInheritanceClause] [TypealiasAssignment] [GenericWhereClause]

TypealiasAssignment:
= Type

```

### 6.7 Initializers and Deinitializers

```text
InitializerDeclaration:
[AttributeList] [DeclarationModifiers] init [?] [!] ( [ParameterList] ) [async] [ThrowsClause] [GenericWhereClause] CodeBlock
[AttributeList] [DeclarationModifiers] init [?] [!] ( [ParameterList] ) [async] rethrows [GenericWhereClause] CodeBlock

DeinitializerDeclaration:
[AttributeList] [DeclarationModifiers] deinit CodeBlock

```

### 6.8 Extensions and Subscripts

```text
ExtensionDeclaration:
[AttributeList] [AccessLevelModifier] extension TypeIdentifier [TypeInheritanceClause] [GenericWhereClause] '{' {ExtensionMember} '}'

ExtensionMember:
Declaration
CompilerControlStatement

SubscriptDeclaration:
[AttributeList] [DeclarationModifiers] subscript ( [ParameterList] ) -> Type [GenericWhereClause] SubscriptResult

SubscriptResult:
GetterSetterBlock
CodeBlock

```

### 6.9 Operators, Precedence Groups, and Macros

```text
OperatorDeclaration:
[AttributeList] prefix operator Operator
[AttributeList] postfix operator Operator
[AttributeList] infix operator Operator [InfixOperatorGroup]

InfixOperatorGroup:
: PrecedenceGroupName

PrecedenceGroupName:
Identifier

PrecedenceGroupDeclaration:
precedencegroup PrecedenceGroupName '{' {PrecedenceGroupAttribute} '}'

PrecedenceGroupAttribute:
PrecedenceGroupRelation
PrecedenceGroupAssignment
PrecedenceGroupAssociativity

PrecedenceGroupRelation:
higherThan : PrecedenceGroupNames
lowerThan : PrecedenceGroupNames

PrecedenceGroupNames:
PrecedenceGroupName {, PrecedenceGroupName}

PrecedenceGroupAssignment:
assignment : BooleanLiteral

PrecedenceGroupAssociativity:
associativity : (one of) left right none

MacroDeclaration:
[AttributeList] [DeclarationModifiers] macro MacroName [GenericParameterClause] FunctionSignature = MacroExpansion [GenericWhereClause]
[AttributeList] [DeclarationModifiers] macro MacroName [GenericParameterClause] FunctionSignature [GenericWhereClause]

MacroName:
Identifier

MacroExpansion:
TypeIdentifier

```

---

## 7. Attributes

```text
AttributeList:
Attribute {Attribute}

Attribute:
@ AttributeName [AttributeArgumentClause]

AttributeName:
Identifier
TypeIdentifier

AttributeArgumentClause:
( [BalancedTokens] )

BalancedTokens:
BalancedToken {BalancedToken}

BalancedToken:
( [BalancedTokens] )
'[' [BalancedTokens] ']'
'{' [BalancedTokens] '}'
(any sequence of valid Swift tokens except (, ), [, ], {, or })

```

---

## 8. Patterns

```text
Pattern:
WildcardPattern [TypeAnnotation]
IdentifierPattern [TypeAnnotation]
ValueBindingPattern
TuplePattern [TypeAnnotation]
EnumCasePattern
OptionalPattern
TypeCastingPattern
ExpressionPattern

WildcardPattern:
_

IdentifierPattern:
Identifier

ValueBindingPattern:
var Pattern
let Pattern

TuplePattern:
( [TuplePatternElementList] )

TuplePatternElementList:
TuplePatternElement {, TuplePatternElement}

TuplePatternElement:
Pattern
Identifier : Pattern

EnumCasePattern:
[TypeIdentifier] . EnumCaseName [TuplePattern]

OptionalPattern:
IdentifierPattern ?

TypeCastingPattern:
is Type
Pattern as Type

ExpressionPattern:
Expression

TypeAnnotation:
: Type

```

---

## 9. Generic Parameters and Arguments

```text
GenericParameterClause:
< GenericParameterList >

GenericParameterList:
GenericParameter {, GenericParameter}

GenericParameter:
[each] TypeName [TypeInheritanceClause]

GenericArgumentClause:
< GenericArgumentList >

GenericArgumentList:
GenericArgument {, GenericArgument}

GenericArgument:
Type
each Type

GenericWhereClause:
where RequirementList

RequirementList:
Requirement {, Requirement}

Requirement:
ConformanceRequirement
SameTypeRequirement

ConformanceRequirement:
TypeIdentifier : [~] TypeIdentifier
TypeIdentifier : ProtocolCompositionType

SameTypeRequirement:
TypeIdentifier == Type

TypeInheritanceClause:
: InheritanceList

InheritanceList:
InheritanceItem {, InheritanceItem}

InheritanceItem:
[~] TypeIdentifier

```