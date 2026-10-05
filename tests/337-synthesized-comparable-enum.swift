// Comparable synthesized for an enum without payloads: cases order as declared.
enum Reach: Comparable { case none, local, global }
print(Reach.none < .local, Reach.global < .local, [Reach.global, .none, .local].sorted(), max(Reach.local, .global))
