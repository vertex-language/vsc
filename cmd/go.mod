module github.com/vertex-language/vsc/cmd

go 1.25.0

require github.com/vertex-language/vsc v0.0.0

require (
	dario.cat/mergo v1.0.0 // indirect
	github.com/Microsoft/go-winio v0.6.2 // indirect
	github.com/ProtonMail/go-crypto v1.1.6 // indirect
	github.com/cloudflare/circl v1.6.3 // indirect
	github.com/cyphar/filepath-securejoin v0.6.1 // indirect
	github.com/emirpasic/gods v1.18.1 // indirect
	github.com/go-git/gcfg v1.5.1-0.20230307220236-3a3c6141e376 // indirect
	github.com/go-git/go-billy/v5 v5.9.0 // indirect
	github.com/go-git/go-git/v5 v5.19.2 // indirect
	github.com/golang/groupcache v0.0.0-20241129210726-2c02b8208cf8 // indirect
	github.com/jbenet/go-context v0.0.0-20150711004518-d14ea06fba99 // indirect
	github.com/kevinburke/ssh_config v1.2.0 // indirect
	github.com/klauspost/cpuid/v2 v2.3.0 // indirect
	github.com/pjbgf/sha1cd v0.6.0 // indirect
	github.com/sergi/go-diff v1.3.2-0.20230802210424-5b0b94c5c0d3 // indirect
	github.com/skeema/knownhosts v1.3.1 // indirect
	github.com/vertex-language/air v0.0.0
	github.com/vertex-language/amd64 v0.0.0 // indirect
	github.com/vertex-language/amdgpu v0.0.0 // indirect
	github.com/vertex-language/arm64 v0.0.0 // indirect
	github.com/vertex-language/asm v0.0.0 // indirect
	github.com/vertex-language/elf v0.0.0 // indirect
	github.com/vertex-language/i386 v0.0.0 // indirect
	github.com/vertex-language/ir v0.0.0 // indirect
	github.com/vertex-language/ir/lower v0.0.0 // indirect
	github.com/vertex-language/macho v0.0.0 // indirect
	github.com/vertex-language/objv v0.0.0-00010101000000-000000000000 // indirect
	github.com/vertex-language/pe v0.0.0 // indirect
	github.com/vertex-language/ptx v0.0.0 // indirect
	github.com/vertex-language/vcc v0.0.0 // indirect
	github.com/vertex-language/vcx v0.0.0 // indirect
	github.com/vertex-language/vsc/build v0.0.0 // indirect
	github.com/vertex-language/vsc/stdlib v0.0.0 // indirect
	github.com/xanzy/ssh-agent v0.3.3 // indirect
	golang.org/x/crypto v0.53.0 // indirect
	golang.org/x/net v0.56.0 // indirect
	golang.org/x/sys v0.46.0 // indirect
	gopkg.in/warnings.v0 v0.1.2 // indirect
)

replace (
	github.com/vertex-language/amd64 => ../../amd64
	github.com/vertex-language/arm64 => ../../arm64
	github.com/vertex-language/asm => ../../asm
	github.com/vertex-language/elf => ../../elf
	github.com/vertex-language/i386 => ../../i386
	github.com/vertex-language/ir => ../../ir
	github.com/vertex-language/ir/lower => ../../ir/lower
	github.com/vertex-language/macho => ../../macho
	github.com/vertex-language/objv => ../../objv
	github.com/vertex-language/pe => ../../pe
	github.com/vertex-language/vcc => ../../vcc
	github.com/vertex-language/vcx => ../../vcx
	github.com/vertex-language/vsc => ..
	github.com/vertex-language/vsc/build => ../build
	github.com/vertex-language/vsc/stdlib => ../stdlib
)

replace github.com/vertex-language/amdgpu => ../../amdgpu

replace github.com/vertex-language/ptx => ../../ptx

replace github.com/vertex-language/air => ../../air
