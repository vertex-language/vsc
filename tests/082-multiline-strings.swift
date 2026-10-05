// Multi-line strings: indentation stripped, and a line continued with \.
let text = """
    first line
      indented line
    joined \
    here
    "quotes" need no escape
    """
print(text)
print(text.count)
