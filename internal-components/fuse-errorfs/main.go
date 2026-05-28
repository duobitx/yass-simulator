// fuse-errorfs is a minimal FUSE filesystem that returns EIO on every
// operation. It is mounted by world-controller over an FsNode's volume
// to simulate a dead storage device (HardwareEventDiskFailure — see
// yass-docs/hardware-events-spec.md §9.4).
//
// Usage: fuse-errorfs <mountpoint>
//
// Unmount with: fusermount -u <mountpoint>
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"syscall"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
)

type errorFS struct{}

func (errorFS) Root() (fs.Node, error) { return errorDir{}, nil }

// errorDir is the (only) inode the FS exposes. Every callback returns
// EIO so engine and agent see exactly the symptom of a dead disk.
type errorDir struct{}

func (errorDir) Attr(_ context.Context, a *fuse.Attr) error {
	a.Mode = os.ModeDir | 0555
	return nil
}

func (errorDir) Lookup(_ context.Context, _ string) (fs.Node, error) {
	return nil, syscall.EIO
}

func (errorDir) ReadDirAll(_ context.Context) ([]fuse.Dirent, error) {
	return nil, syscall.EIO
}

func (errorDir) Create(_ context.Context, _ *fuse.CreateRequest, _ *fuse.CreateResponse) (fs.Node, fs.Handle, error) {
	return nil, nil, syscall.EIO
}

func (errorDir) Mkdir(_ context.Context, _ *fuse.MkdirRequest) (fs.Node, error) {
	return nil, syscall.EIO
}

func (errorDir) Remove(_ context.Context, _ *fuse.RemoveRequest) error {
	return syscall.EIO
}

func main() {
	flag.Parse()
	if flag.NArg() != 1 {
		log.Fatal("usage: fuse-errorfs <mountpoint>")
	}
	mp := flag.Arg(0)
	c, err := fuse.Mount(mp,
		fuse.FSName("errorfs"),
		fuse.Subtype("errorfs"),
		fuse.AllowOther(),
	)
	if err != nil {
		log.Fatalf("mount %s: %v", mp, err)
	}
	defer c.Close()
	if err := fs.Serve(c, errorFS{}); err != nil {
		log.Fatalf("serve: %v", err)
	}
}
