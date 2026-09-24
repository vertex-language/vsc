// Dictionary: lookup returns an Optional; insert, update and remove.
var ages = ["ann": 31, "bob": 25]
print(ages["ann"] as Any, ages["zed"] as Any, ages.count)
ages["cy"] = 40
ages["bob"] = 26
let old = ages.updateValue(32, forKey: "ann")
ages["cy"] = nil
print(old as Any, ages.removeValue(forKey: "nobody") as Any, ages.count)
for k in ages.keys.sorted() { print(k, ages[k]!) }
