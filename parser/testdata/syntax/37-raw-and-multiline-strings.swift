// §2.3.2 Deep Raw and Multiline Strings

let singlePoundRaw = #"Hello "World", no escapes here: \n, \t"#
let twoPoundRaw = ##"Can contain #" quotes and lone # without issue"##
let threePoundRaw = ###"Even ##" is completely raw"###

let multilineRaw = ##"""
    Line 1 with "quotes" and \n
    Line 2 with lone #
    Interpolation: \##(10 + 20)
    """##

let rawWithInterpolation = #"Result: \#(40 + 2), and raw text"#

let escapedNewlineString = """
    This line continues \
    onto the next line \
    without line breaks.
    """

let unicodeEscapes = "Icons: \u{1F600} \u{2764} \u{1F4BB}"

let emptyRaw = #""#
let emptyMultilineRaw = #"""
"""#
