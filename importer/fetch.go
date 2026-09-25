package importer

import (
	"fmt"
	"os"
	"path/filepath"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

// Options say how much a resolve is allowed to do.
type Options struct {
	// Ref is the version to take where the path pins none: what the
	// building module's vs.mod requires of it. Empty is the default branch.
	Ref string

	// Cache is where packages are kept; CacheDir() when empty.
	Cache string
	// Offline forbids the network. A package already in the cache is
	// used; one that is not is an error rather than a download.
	Offline bool
	// Update fetches a package again even if the cache holds it.
	Update bool
	// Log receives one line the first time a package is fetched, so a
	// build that pauses on the network says why. Nil to say nothing.
	Log func(string)
}

func (o Options) cache() string {
	if o.Cache != "" {
		return o.Cache
	}
	return CacheDir()
}

// Fetch is m's checkout, cloning it into the cache if it is not there.
//
// A package already in the cache is used as it stands: a build that has
// fetched once does no network I/O, which is what makes the second run of
// `vsc run` as quick as any other. Update asks for it again.
func Fetch(m Module, opts Options) (string, error) {
	dir := PackageDir(m, opts.cache())
	have := isCheckout(dir)

	if have && !opts.Update {
		return dir, nil
	}
	if opts.Offline {
		if have {
			return dir, nil
		}
		return "", fmt.Errorf("package '%s' is not in the cache and fetching is off: "+
			"run without -offline, or put it at %s", m.Path, dir)
	}
	if have {
		if err := os.RemoveAll(dir); err != nil {
			return "", fmt.Errorf("cannot replace %s: %w", dir, err)
		}
	}
	if opts.Log != nil {
		opts.Log("fetching " + m.Repo)
	}
	if err := clone(m, dir); err != nil {
		return "", err
	}
	return dir, nil
}

// clone writes m's repository into dir.
//
// It clones beside dir and renames when it has finished, so an
// interrupted fetch cannot leave half a package in the cache for the next
// build to compile.
func clone(m Module, dir string) error {
	parent := filepath.Dir(dir)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("cannot make the package cache at %s: %w", parent, err)
	}
	tmp, err := os.MkdirTemp(parent, ".fetch-*")
	if err != nil {
		return fmt.Errorf("cannot make the package cache at %s: %w", parent, err)
	}
	defer os.RemoveAll(tmp)

	// One commit of one branch: a build reads the source, and no history
	// is worth the download.
	o := &git.CloneOptions{URL: m.Repo, Depth: 1, SingleBranch: true, Tags: git.NoTags}
	if m.Ref != "" {
		o.ReferenceName = plumbing.NewBranchReferenceName(m.Ref)
	}
	if _, err := git.PlainClone(tmp, false, o); err != nil {
		if m.Ref != "" {
			// A ref that is not a branch may still be a tag.
			o.ReferenceName = plumbing.NewTagReferenceName(m.Ref)
			if _, tagErr := git.PlainClone(tmp, false, o); tagErr == nil {
				return finish(tmp, dir)
			}
		}
		return fmt.Errorf("cannot fetch '%s' from %s: %w", m.Path, m.Repo, err)
	}
	return finish(tmp, dir)
}

// finish moves a finished clone into place. Another build may have
// fetched the same package meanwhile, and its copy is as good as this
// one, so losing the race is not an error.
func finish(tmp, dir string) error {
	if err := os.Rename(tmp, dir); err != nil {
		if isCheckout(dir) {
			return nil
		}
		return fmt.Errorf("cannot put the fetched package at %s: %w", dir, err)
	}
	return nil
}

// isCheckout reports whether dir holds a fetched package.
func isCheckout(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil && (info.IsDir() || info.Mode().IsRegular())
}
