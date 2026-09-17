package build_test

import (
	"io/fs"
	"testing"

	"github.com/vertex-language/vcx"

	"github.com/vertex-language/vsc/stdlib"
)

// TestTheABIIsOneContract holds the runtime's structs and the numbers
// the compiler reads them by together.
//
// The layouts are written twice, once in C++ in abi.h and once in Go in
// stdlib, because the runtime is compiled from the first and the
// compiler lowers against the second. So the first is compiled here,
// by the same vcx that compiles the runtime, and every offset the
// compiler depends on is read out of vcx's layout and compared. A
// field moved on one side only is a failure here instead of a program
// that misreads its own objects.
func TestTheABIIsOneContract(t *testing.T) {
	for _, target := range []string{"aarch64-macos", "x86_64-windows"} {
		t.Run(target, func(t *testing.T) {
			include, err := fs.Sub(stdlib.Runtime(), "include")
			if err != nil {
				t.Fatal(err)
			}
			src := []byte("#include \"vertex/abi.h\"\n")
			c := &vcx.Compiler{Target: target, Std: vcx.Cxx23, Freestanding: true,
				IncludeFS: []vcx.SystemInclude{{Name: "<vertex>", FS: include}}}
			records, diags, err := c.Layout(vcx.Text("abi.cpp", src))
			if err != nil || vcx.HasErrors(diags) {
				t.Fatalf("abi.h does not compile: %v %v", err, diags)
			}
			layout := map[string]vcx.RecordLayout{}
			for _, r := range records {
				layout[r.Name] = r
			}
			field := func(record, name string) int64 {
				t.Helper()
				r, ok := layout[record]
				if !ok {
					t.Fatalf("abi.h has no struct %s", record)
				}
				for _, f := range r.Fields {
					if f.Name == name {
						return f.Offset
					}
				}
				t.Fatalf("struct %s has no field %s", record, name)
				return 0
			}
			size := func(record string) int64 {
				t.Helper()
				r, ok := layout[record]
				if !ok {
					t.Fatalf("abi.h has no struct %s", record)
				}
				return r.Size
			}
			check := func(what string, got, want int64) {
				t.Helper()
				if got != want {
					t.Errorf("%s: abi.h says %d, stdlib says %d", what, got, want)
				}
			}

			check("HeapObject size", size("HeapObject"), stdlib.HeaderBytes)
			check("ValueWitnessTable.destroy", field("ValueWitnessTable", "destroy"), stdlib.WitnessDestroy)
			check("ValueWitnessTable.flags", field("ValueWitnessTable", "flags"), stdlib.WitnessFlags)
			check("FullMetadata.metadata", field("FullMetadata", "metadata"), stdlib.MetadataOffset)
			check("AnyExistential.buffer", field("AnyExistential", "buffer"), stdlib.ExistentialBuffer)
			check("AnyExistential.type", field("AnyExistential", "type"), stdlib.ExistentialMetadata)
			check("String size", size("String"), stdlib.StringBytes)
			check("String.object", field("String", "object"), stdlib.StringObjectWord*8)
			check("ArrayStorage size", size("ArrayStorage"), stdlib.ArrayElements)
		})
	}
}
