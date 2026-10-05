package cli

import (
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/vertex-language/vsc"
	"github.com/vertex-language/vsc/build"
	"github.com/vertex-language/vsc/build/sysroot"
	"github.com/vertex-language/vsc/importer"
)

// cmdEnv prints target, host, symbol, and toolchain resolution information.
func cmdEnv(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("env", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var c common
	c.register(fs)
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	target, err := c.resolve()
	if err != nil {
		fmt.Fprintln(stderr, "vsc:", err)
		return exitUsage
	}

	host := vsc.HostName()
	if host == "" {
		host = "(not a target vsc models)"
	}
	fmt.Fprintf(stdout, "target\t%s\n", c.target)
	fmt.Fprintf(stdout, "host\t%s\n", host)
	fmt.Fprintf(stdout, "use\t%s\n", target.Use())
	fmt.Fprintf(stdout, "module\t%s\n", c.module)
	fmt.Fprintf(stdout, "entry\t%s\n", vsc.EntrySymbol(target))
	fmt.Fprintf(stdout, "prefix\t%q\n", vsc.SymbolPrefix(target))
	fmt.Fprintf(stdout, "targets\t%s\n", strings.Join(vsc.Targets(), ", "))
	fmt.Fprintf(stdout, "cache\t%s\n", importer.CacheDir())

	// Resolve library and SDK search paths.
	if sysroot.IsMacOS(c.target) {
		sdk, ok := build.SDK()
		if !ok {
			sdk = "(none found: set SDKROOT, or xcode-select --install)"
		}
		fmt.Fprintf(stdout, "sdk\t%s\n", sdk)
	}
	dirs := build.LibraryDirs(target, false)
	if len(dirs) == 0 {
		fmt.Fprintf(stdout, "libdirs\t(none found)\n")
	}
	for i, dir := range dirs {
		label := "libdirs"
		if i > 0 {
			label = ""
		}
		fmt.Fprintf(stdout, "%s\t%s\n", label, dir)
	}
	if libs := build.DefaultLibraries(target, false); len(libs) > 0 {
		fmt.Fprintf(stdout, "libs\t%s\n", strings.Join(libs, ", "))
	}

	if _, ok := build.Host(); !ok {
		fmt.Fprintln(stdout, "note\tthis machine has no backend; build can emit sil and vir only")
	}
	return exitOK
}
