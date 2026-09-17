// Package importer resolves an import path that names a package this
// machine does not have yet, fetches it, and says where its source is.
//
// `import "net/tcp"` names a package rather than a file, as Go's imports
// do. A path naming no host is the standard library's: its first segment is
// a repository under github.com/vertex-language, and the rest a folder in it,
// so net/tcp is the tcp folder of github.com/vertex-language/net. A path
// naming a host names its repository itself, as github.com/you/thing does.
//
// A package of Vertex alone is just its folders. A manifest, package.vs, is
// only needed where a package builds C, C++ or Objective-C beside them.
//
// Before anything is fetched, the checkout the importing file is in answers
// for itself (see Local): a package's tests and examples, and one of its
// folders importing another, read the package from disk.
//
// A package is fetched once into the package cache and read from there
// afterwards, so a second build of the same program does no network I/O
// at all. Cloning is go-git rather than the git program, for the same
// reason the linker is this project's own: a Vertex toolchain should not
// need anything else installed to build a program.
//
// Nothing here compiles anything. Resolve hands back a directory, and the
// caller builds what is in it the way it builds any package folder.
package importer
