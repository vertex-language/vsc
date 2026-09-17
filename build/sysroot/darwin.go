package sysroot

import "strings"

// SDK returns the path to the macOS SDK using $SDKROOT, xcrun, or the default CLT path.
// A nil Host defaults to osHost.
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
