// Package build provides backend lowering from Vertex IR (VIR) to native
// object files and links objects into native executables.
//
// Object lowers a VIR module into machine object bytes, and Executable
// links object inputs into native binaries (Mach-O, PE/COFF) without external
// toolchain dependencies.
package build
