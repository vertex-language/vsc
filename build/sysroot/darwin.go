package sysroot

import "strings"

// The macOS SDK: where a Mach-O link finds the platform's libraries.
//
// /usr/lib holds no dylibs on a modern macOS — the shared cache
// replaced them — so what a link actually reads is the .tbd stub
// inside an SDK. Finding the SDK is therefore not a convenience but
// the difference between a link and no link.
//
// The lookup is the one Apple's own tools use, in their order.

// SDK is the macOS SDK to link against, and whether one was found.
//
//  1. $SDKROOT, which xcrun and Xcode-driven builds set, so honouring
//     it means vsc composes under both.
//  2. `xcrun --show-sdk-path`, the authoritative answer. Apple owns
//     the developer-directory walk behind it and changes it between
//     releases, so this asks rather than reimplements.
//  3. The Command Line Tools SDK at its fixed path, for a machine
//     that has the tools but whose xcrun does not answer.
//
// A nil Host is the real machine.
func SDK(h Host) (string, bool) { return darwinSDK(host(h)) }

func darwinSDK(h Host) (string, bool) {
	if sdk := h.Getenv("SDKROOT"); sdk != "" {
		return sdk, true
	}
	if out, err := h.Run("xcrun", "--show-sdk-path"); err == nil {
		if sdk := strings.TrimSpace(out); sdk != "" {
			return sdk, true
		}
	}
	const clt = "/Library/Developer/CommandLineTools/SDKs/MacOSX.sdk"
	if h.IsDir(clt) {
		return clt, true
	}
	return "", false
}
