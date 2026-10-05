// NaN is unequal to itself; infinities order and print.
let nan = Double.nan
let inf = Double.infinity
print(nan == nan, nan != nan, nan < 1, nan.isNaN)
print(inf, -inf, inf > 1e308, 1 / inf, inf - inf)
print(Double.greatestFiniteMagnitude, Double.leastNonzeroMagnitude)
