package token

// Kind identifies the lexical class of a token.
type Kind uint8

const (
	ILLEGAL Kind = iota
	EOF
	COMMENT

	// IDENT covers every spelling of Identifier: a plain
	// IdentifierHead run, a `backtick-escaped` one (FlagEscaped), and
	// the $ forms — the implicit parameter name $0 and the property
	// wrapper projection $value.
	IDENT

	literal_beg
	INT_LIT   // a Binary, Octal, Decimal or Hexadecimal literal, undecoded
	FLOAT_LIT // a FloatingPointLiteral, undecoded
	REGEX_LIT // a whole RegexLiteral, delimiters included; the pattern is opaque

	// A string literal is not one token. The grammar admits an
	// Expression inside it, so the scanner opens the literal, hands
	// out its text a segment at a time, and lets the interpolations
	// arrive as ordinary tokens between BACKSLASH LPAREN and RPAREN:
	//
	//	"a\(b)c"  →  STRING_QUOTE STRING_SEGMENT BACKSLASH LPAREN
	//	             IDENT RPAREN STRING_SEGMENT STRING_QUOTE
	//
	// Both quotes of a literal are STRING_QUOTE (or, for a """
	// literal, MULTILINE_STRING_QUOTE); the pound runs of a raw
	// literal are POUND_DELIM tokens outside them. A segment is
	// undecoded: escape sequences stay as written.
	STRING_QUOTE
	MULTILINE_STRING_QUOTE
	STRING_SEGMENT
	POUND_DELIM
	literal_end

	punct_beg
	LPAREN    // (
	RPAREN    // )
	LSQUARE   // [
	RSQUARE   // ]
	LBRACE    // {
	RBRACE    // }
	COMMA     // ,
	COLON     // :
	SEMI      // ;
	AT        // @
	POUND     // # — the macro expansion sigil; the # of a directive is a POUND_* kind
	BACKSLASH // \ — a key-path root, or the head of an interpolation

	// The reserved operators. Each is a run of operator characters
	// that the grammar spells out itself, so it cannot be redeclared
	// by a precedencegroup and does not arrive as OPER_*.
	ASSIGN           // =
	ARROW            // ->
	PERIOD           // . — a member access: bound on both sides, or on neither
	PERIOD_PREFIX    // . — an implicit member reference: bound on the right only
	AMP_PREFIX       // & — an InOutExpression, not the binary operator
	QUESTION_POSTFIX // ? — optional chaining, or an OptionalType
	QUESTION_INFIX   // ? — the ConditionalOperator's head
	EXCLAIM_POSTFIX  // ! — a ForcedValueExpression, or an ImplicitlyUnwrappedOptionalType
	punct_end

	// A general operator: the spelling is the span, and the meaning
	// is a precedencegroup's business. The three kinds are the
	// scanner's reading of the whitespace around the run, which is
	// the whole of what position can be known lexically.
	oper_beg
	OPER_PREFIX
	OPER_BINARY
	OPER_POSTFIX
	oper_end

	// The reserved words. A word outside this list is an IDENT, even
	// where the grammar writes it as a terminal: get, set, some,
	// async, actor, weak and their kin are contextual, and a program
	// may name a variable after any of them. IsContextualKeyword
	// lists them; the parser matches those by spelling.
	keyword_beg
	AS              // as
	ASSOCIATEDTYPE  // associatedtype
	ANY             // Any
	BREAK           // break
	CASE            // case
	CATCH           // catch
	CLASS           // class
	CONTINUE        // continue
	DEFAULT         // default
	DEFER           // defer
	DEINIT          // deinit
	DO              // do
	ELSE            // else
	ENUM            // enum
	EXTENSION       // extension
	FALLTHROUGH     // fallthrough
	FALSE           // false
	FILEPRIVATE     // fileprivate
	FOR             // for
	FUNC            // func
	GUARD           // guard
	IF              // if
	IMPORT          // import
	IN              // in
	INIT            // init
	INOUT           // inout
	INTERNAL        // internal
	IS              // is
	LET             // let
	NIL             // nil
	OPERATOR        // operator
	PRECEDENCEGROUP // precedencegroup
	PRIVATE         // private
	PROTOCOL        // protocol
	PUBLIC          // public
	REPEAT          // repeat
	RETHROWS        // rethrows
	RETURN          // return
	SELF            // self
	SELF_TYPE       // Self
	STATIC          // static
	STRUCT          // struct
	SUBSCRIPT       // subscript
	SUPER           // super
	SWITCH          // switch
	THROW           // throw
	THROWS          // throws
	TRUE            // true
	TRY             // try
	TYPEALIAS       // typealias
	VAR             // var
	WHERE           // where
	WHILE           // while
	UNDERSCORE      // _ — the WildcardPattern and the omitted ArgumentLabel
	keyword_end

	// The # words. Unlike the reserved words these are unambiguous:
	// nothing else may follow a # with no space, so a spelling that
	// is not one of these is a MacroExpansion — POUND then IDENT.
	pound_beg
	POUND_IF             // #if
	POUND_ELSE           // #else
	POUND_ELSEIF         // #elseif
	POUND_ENDIF          // #endif
	POUND_SOURCELOCATION // #sourceLocation
	POUND_ERROR          // #error
	POUND_WARNING        // #warning
	POUND_AVAILABLE      // #available
	POUND_UNAVAILABLE    // #unavailable
	POUND_SELECTOR       // #selector
	POUND_KEYPATH        // #keyPath
	POUND_FILE           // #file
	POUND_FILEID         // #fileID
	POUND_FILEPATH       // #filePath
	POUND_LINE           // #line
	POUND_COLUMN         // #column
	POUND_FUNCTION       // #function
	POUND_DSOHANDLE      // #dsohandle
	POUND_COLORLITERAL   // #colorLiteral
	POUND_FILELITERAL    // #fileLiteral
	POUND_IMAGELITERAL   // #imageLiteral
	pound_end
)

var names = [...]string{
	ILLEGAL: "ILLEGAL",
	EOF:     "EOF",
	COMMENT: "COMMENT",

	IDENT:                  "IDENT",
	INT_LIT:                "INT_LIT",
	FLOAT_LIT:              "FLOAT_LIT",
	REGEX_LIT:              "REGEX_LIT",
	STRING_QUOTE:           `"`,
	MULTILINE_STRING_QUOTE: `"""`,
	STRING_SEGMENT:         "STRING_SEGMENT",
	POUND_DELIM:            "#",

	LPAREN:    "(",
	RPAREN:    ")",
	LSQUARE:   "[",
	RSQUARE:   "]",
	LBRACE:    "{",
	RBRACE:    "}",
	COMMA:     ",",
	COLON:     ":",
	SEMI:      ";",
	AT:        "@",
	POUND:     "#",
	BACKSLASH: `\`,

	ASSIGN:           "=",
	ARROW:            "->",
	PERIOD:           ".",
	PERIOD_PREFIX:    ".",
	AMP_PREFIX:       "&",
	QUESTION_POSTFIX: "?",
	QUESTION_INFIX:   "?",
	EXCLAIM_POSTFIX:  "!",

	OPER_PREFIX:  "prefix operator",
	OPER_BINARY:  "binary operator",
	OPER_POSTFIX: "postfix operator",

	AS:              "as",
	ASSOCIATEDTYPE:  "associatedtype",
	ANY:             "Any",
	BREAK:           "break",
	CASE:            "case",
	CATCH:           "catch",
	CLASS:           "class",
	CONTINUE:        "continue",
	DEFAULT:         "default",
	DEFER:           "defer",
	DEINIT:          "deinit",
	DO:              "do",
	ELSE:            "else",
	ENUM:            "enum",
	EXTENSION:       "extension",
	FALLTHROUGH:     "fallthrough",
	FALSE:           "false",
	FILEPRIVATE:     "fileprivate",
	FOR:             "for",
	FUNC:            "func",
	GUARD:           "guard",
	IF:              "if",
	IMPORT:          "import",
	IN:              "in",
	INIT:            "init",
	INOUT:           "inout",
	INTERNAL:        "internal",
	IS:              "is",
	LET:             "let",
	NIL:             "nil",
	OPERATOR:        "operator",
	PRECEDENCEGROUP: "precedencegroup",
	PRIVATE:         "private",
	PROTOCOL:        "protocol",
	PUBLIC:          "public",
	REPEAT:          "repeat",
	RETHROWS:        "rethrows",
	RETURN:          "return",
	SELF:            "self",
	SELF_TYPE:       "Self",
	STATIC:          "static",
	STRUCT:          "struct",
	SUBSCRIPT:       "subscript",
	SUPER:           "super",
	SWITCH:          "switch",
	THROW:           "throw",
	THROWS:          "throws",
	TRUE:            "true",
	TRY:             "try",
	TYPEALIAS:       "typealias",
	VAR:             "var",
	WHERE:           "where",
	WHILE:           "while",
	UNDERSCORE:      "_",

	POUND_IF:             "#if",
	POUND_ELSE:           "#else",
	POUND_ELSEIF:         "#elseif",
	POUND_ENDIF:          "#endif",
	POUND_SOURCELOCATION: "#sourceLocation",
	POUND_ERROR:          "#error",
	POUND_WARNING:        "#warning",
	POUND_AVAILABLE:      "#available",
	POUND_UNAVAILABLE:    "#unavailable",
	POUND_SELECTOR:       "#selector",
	POUND_KEYPATH:        "#keyPath",
	POUND_FILE:           "#file",
	POUND_FILEID:         "#fileID",
	POUND_FILEPATH:       "#filePath",
	POUND_LINE:           "#line",
	POUND_COLUMN:         "#column",
	POUND_FUNCTION:       "#function",
	POUND_DSOHANDLE:      "#dsohandle",
	POUND_COLORLITERAL:   "#colorLiteral",
	POUND_FILELITERAL:    "#fileLiteral",
	POUND_IMAGELITERAL:   "#imageLiteral",
}

// String returns the keyword or punctuator spelling, or the class
// name for kinds with no fixed spelling (IDENT, INT_LIT, OPER_*, …).
func (k Kind) String() string {
	if int(k) < len(names) && names[k] != "" {
		return names[k]
	}
	return "Kind(" + itoa(int(k)) + ")"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [8]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

var keywords = func() map[string]Kind {
	m := make(map[string]Kind, keyword_end-keyword_beg-1)
	for k := keyword_beg + 1; k < keyword_end; k++ {
		m[names[k]] = k
	}
	return m
}()

var poundKeywords = func() map[string]Kind {
	m := make(map[string]Kind, pound_end-pound_beg-1)
	for k := pound_beg + 1; k < pound_end; k++ {
		m[names[k][1:]] = k // keyed without the '#'
	}
	return m
}()

// Lookup maps an identifier spelling to its reserved-word kind, or
// IDENT. A backtick-escaped spelling is an IDENT whatever it holds,
// so the scanner does not consult Lookup for one.
func Lookup(name string) Kind {
	if k, ok := keywords[name]; ok {
		return k
	}
	return IDENT
}

// LookupPound maps the word after a '#' to its kind, or POUND — in
// which case the '#' opens a MacroExpansion and the word is an
// ordinary IDENT.
func LookupPound(word string) Kind {
	if k, ok := poundKeywords[word]; ok {
		return k
	}
	return POUND
}

// contextual holds the words the grammar writes as terminals that are
// not reserved: they are identifiers everywhere else, so the parser
// matches them by spelling and a program may still name something
// after one. `var set = 0` is a variable named set.
var contextual = map[string]bool{
	// declaration modifiers and their arguments
	"actor": true, "assignment": true, "associativity": true, "borrowing": true,
	"consuming": true, "convenience": true, "didSet": true, "distributed": true,
	"dynamic": true, "each": true, "final": true, "get": true, "higherThan": true,
	"indirect": true, "infix": true, "isolated": true, "lazy": true,
	"lowerThan": true, "macro": true, "mutating": true, "nonisolated": true,
	"nonmutating": true, "none": true, "nonsending": true, "open": true,
	"optional": true, "override": true, "package": true, "postfix": true,
	"prefix": true, "required": true, "safe": true, "set": true,
	"unowned": true, "unsafe": true, "weak": true, "willSet": true,
	"left": true, "right": true,
	// expression and type positions
	"any": true, "async": true, "await": true, "borrow": true, "consume": true,
	"copy": true, "discard": true, "of": true, "sending": true, "some": true,
	// metatypes
	"Protocol": true, "Type": true,
	// accessor and attribute arguments
	"file": true, "getter": true, "line": true, "setter": true,
	// compilation conditions
	"arch": true, "canImport": true, "compiler": true, "hasAttribute": true,
	"hasFeature": true, "os": true, "swift": true, "targetEnvironment": true,
	"_compiler_version": true, "_endian": true, "_hasAtomicBitWidth": true,
	"_pointerBitWidth": true, "_ptrauth": true, "_runtime": true,
	"_underlyingVersion": true, "_version": true,
	// the underscored spellings. They are SPI, and every module
	// interface in an SDK is written with them.
	"__consuming": true, "__owned": true, "__shared": true,
	"_const": true, "_local": true, "_modify": true, "_read": true,
	"unsafeAddress": true, "unsafeMutableAddress": true, "yield": true,
}

// IsContextualKeyword reports whether a spelling is one the grammar
// gives meaning to in some position while leaving it an identifier
// everywhere else.
func IsContextualKeyword(name string) bool { return contextual[name] }

func (k Kind) IsLiteral() bool  { return literal_beg < k && k < literal_end }
func (k Kind) IsPunct() bool    { return punct_beg < k && k < punct_end }
func (k Kind) IsOperator() bool { return oper_beg < k && k < oper_end }
func (k Kind) IsKeyword() bool  { return keyword_beg < k && k < keyword_end }
func (k Kind) IsPound() bool    { return pound_beg < k && k < pound_end }
