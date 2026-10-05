// Package buildcache is the compiler's content-addressed object cache.
//
// An entry is a result the compiler would otherwise have to produce
// again -- the runtime's object, a package's object -- filed under the
// hash of everything that went into it. The first input of every key is
// the compiler itself (CompilerID), so a rebuilt vsc never reads what an
// older one wrote: a change anywhere in the compiler, the backend or the
// embedded runtime source changes the executable, and with it every key.
//
// It is Go's build cache in miniature. Entries are written to a temporary
// file and renamed into place, so a reader sees a whole entry or none,
// and two builds racing to write the same key write the same bytes.
// Everything under it can be deleted at any time.
//
// VSC_CACHE=off turns it off: every Get misses and Put does nothing.
package buildcache

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

// Env is the variable that turns the cache off ("off").
const Env = "VSC_CACHE"

// Dir is where entries are kept: VERTEXCACHE/obj where that is set, and
// vertex/obj in this machine's cache directory otherwise -- beside the
// fetched packages and the native objects.
func Dir() string {
	if dir := os.Getenv("VERTEXCACHE"); dir != "" {
		return filepath.Join(dir, "obj")
	}
	if base, err := os.UserCacheDir(); err == nil {
		return filepath.Join(base, "vertex", "obj")
	}
	return filepath.Join(os.TempDir(), "vertex-cache", "obj")
}

// Enabled reports whether the cache is in use: not turned off, and the
// compiler knows what it is. Without an identity a key could not tell
// two compilers apart, and a stale entry would be read as a fresh one.
func Enabled() bool { return os.Getenv(Env) != "off" && CompilerID() != "unknown" }

// A Key names an entry.
type Key [sha256.Size]byte

func (k Key) String() string { return hex.EncodeToString(k[:]) }

// A Hasher builds a key from a sequence of inputs. Each input is framed
// by its length, so ("ab", "c") and ("a", "bc") hash differently.
type Hasher struct {
	h interface {
		io.Writer
		Sum([]byte) []byte
	}
}

// New starts a key for a kind of entry ("runtime", "package"), with the
// compiler's identity as its first input.
func New(kind string) *Hasher {
	h := &Hasher{h: sha256.New()}
	h.String("vsc-buildcache-1")
	h.String(CompilerID())
	h.String(kind)
	return h
}

// Bytes adds one input.
func (h *Hasher) Bytes(b []byte) *Hasher {
	var n [8]byte
	binary.LittleEndian.PutUint64(n[:], uint64(len(b)))
	h.h.Write(n[:])
	h.h.Write(b)
	return h
}

// String adds one input.
func (h *Hasher) String(s string) *Hasher { return h.Bytes([]byte(s)) }

// Strings adds a list, framed by its length.
func (h *Hasher) Strings(ss []string) *Hasher {
	h.String(fmt.Sprint(len(ss)))
	for _, s := range ss {
		h.String(s)
	}
	return h
}

// Key is the key of everything added so far.
func (h *Hasher) Key() Key {
	var k Key
	copy(k[:], h.h.Sum(nil))
	return k
}

func path(k Key) string {
	s := k.String()
	return filepath.Join(Dir(), s[:2], s)
}

// Get returns the entry for k, if there is one.
func Get(k Key) ([]byte, bool) {
	if !Enabled() {
		return nil, false
	}
	data, err := os.ReadFile(path(k))
	if err != nil {
		return nil, false
	}
	return data, true
}

// Put files data under k. A failure to write is not a failure of the
// build -- the next one does the work again -- so it is only returned.
func Put(k Key, data []byte) error {
	if !Enabled() {
		return nil
	}
	p := path(k)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(p), ".tmp-*")
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(f.Name())
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(f.Name())
		return err
	}
	if err := os.Rename(f.Name(), p); err != nil {
		os.Remove(f.Name())
		return err
	}
	return nil
}

// CompilerID is the hash of the running executable: what every key
// starts from. Hashing a 40 MB binary costs ~20 ms, so the answer is
// kept in a stamp file named for the executable's path, size and
// modification time, and a later run with the same binary only stats it.
var CompilerID = sync.OnceValue(compilerID)

func compilerID() string {
	exe, err := os.Executable()
	if err != nil {
		return "unknown"
	}
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	info, err := os.Stat(exe)
	if err != nil {
		return "unknown"
	}
	stamp := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%d\x00%d", exe, info.Size(), info.ModTime().UnixNano())))
	stampPath := filepath.Join(Dir(), "id", hex.EncodeToString(stamp[:16]))
	if id, err := os.ReadFile(stampPath); err == nil && len(id) == 2*sha256.Size {
		return string(id)
	}
	f, err := os.Open(exe)
	if err != nil {
		return "unknown"
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "unknown"
	}
	id := hex.EncodeToString(h.Sum(nil))
	if os.MkdirAll(filepath.Dir(stampPath), 0o755) == nil {
		tmp := stampPath + fmt.Sprintf(".%d", os.Getpid())
		if os.WriteFile(tmp, []byte(id), 0o644) == nil {
			os.Rename(tmp, stampPath)
		}
	}
	return id
}
