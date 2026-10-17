// if case with an enum case pattern, its associated values ignored,
// against an optional.
enum Boot {
    case linux(kernel: String, initrd: String?)
    case efi(String)
}
struct Config { var Boot: Boot? }
for cfg in [Config(Boot: .linux(kernel: "vmlinuz", initrd: nil)), Config(Boot: .efi("fw")), Config(Boot: nil)] {
    if case .linux = cfg.Boot { print("linux") }
    else if case .efi = cfg.Boot { print("efi") }
    else { print("none") }
}
