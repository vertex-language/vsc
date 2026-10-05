// Raw strings: backslashes are literal until matched by the same # count.
print(#"a \n stays, "quotes" too"#)
print(#"but \#(1 + 1) interpolates"#)
print(##"a "# inside"##)
