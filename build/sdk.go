package build

import (
	"os"
	"os/exec"
	"strings"
)

// The macOS SDK: where a Mach-O link finds the platform's libraries.
//
// /usr/lib holds no dylibs on a modern macOS — the shared cache
// replaced them — so what a link actually reads is the .tbd stub
// inside an SDK. Finding the SDK is therefore not a convenience but
// the difference between a link and no link.
//
// The lookup is the one Apple's own tools use, in their order, and is
// vcc's sysroot package reduced to the half a compiler with no
// preprocessor needs. When there is a second hosted target here, this
// grows into its own package the way vcc's did; one function for one
// platform does not need one yet.

// SDK is the macOS SDK this host links against, and whether one was
// found.
//
//  1. $SDKROOT, which xcrun and Xcode-driven builds set, so honouring
//     it means vsc composes under both.
//  2. `xcrun --show-sdk-path`, the authoritative answer. Apple owns
//     the developer-directory walk behind it and changes it between
//     releases, so this asks rather than reimplements.
//  3. The Command Line Tools SDK at its fixed path, for a machine
//     that has the tools but whose xcrun does not answer.
func SDK() (string, bool) {
	if sdk := os.Getenv("SDKROOT"); sdk != "" {
		return sdk, true
	}
	if out, err := exec.Command("xcrun", "--show-sdk-path").Output(); err == nil {
		if sdk := strings.TrimSpace(string(out)); sdk != "" {
			return sdk, true
		}
	}
	const clt = "/Library/Developer/CommandLineTools/SDKs/MacOSX.sdk"
	if st, err := os.Stat(clt); err == nil && st.IsDir() {
		return clt, true
	}
	return "", false
}
