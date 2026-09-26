package sysroot

import (
	"strings"
	"sync"
)

// SDK returns the path to the macOS SDK using $SDKROOT, xcrun, or the default CLT path.
// A nil Host defaults to osHost.
//
// The real machine's answer is looked up once per process: xcrun is a
// subprocess, and a build asks more than once.
func SDK(h Host) (string, bool) { return darwinSDK(host(h)) }

var hostSDK = sync.OnceValues(func() (string, bool) { return lookupSDK(osHost{}) })

func darwinSDK(h Host) (string, bool) {
	if _, real := h.(osHost); real {
		return hostSDK()
	}
	return lookupSDK(h)
}

func lookupSDK(h Host) (string, bool) {
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
