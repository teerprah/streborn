package main

// Linux releases come as archives, .deb and .rpm packages. Only an archive
// installation may update itself by swapping its own binary:
//
//   - the tar.gz from the release: the binary lives wherever the user put it
//     (install.sh copies it to ~/.local/bin), and the in-app swap is right.
//   - a .deb or .rpm: the package manager owns the binary. Swapping it
//     would fail on the root-owned folder anyway, and if it did not, the package
//     record would stay on the old version. Such a copy downloads the next
//     package and hands it to the system's package installer.

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	linuxPkgNone = ""
	linuxPkgDeb  = "deb"
	linuxPkgRPM  = "rpm"
)

// linuxPackageName is shared by build-deb.sh and build-rpm.sh.
const linuxPackageName = "st-reborn"

var (
	linuxPkgOnce sync.Once
	linuxPkg     string
)

// linuxPackage reports how this copy was installed on Linux: linuxPkgDeb, linuxPkgRPM, or
// linuxPkgNone for the tar.gz and every other OS.
func linuxPackage() string {
	linuxPkgOnce.Do(func() {
		if runtime.GOOS != "linux" {
			return
		}
		exe, err := os.Executable()
		if err != nil {
			return
		}
		if r, e := filepath.EvalSymlinks(exe); e == nil {
			exe = r
		}
		linuxPkg = detectLinuxPackage(exe, "/var/lib/dpkg/info")
		if linuxPkg == linuxPkgNone {
			linuxPkg = detectRPMPackage(exe, rpmOwner)
		}
	})
	return linuxPkg
}

// detectLinuxPackage is linuxPackage with every outside input passed in, so the
// decision can be tested anywhere. A .deb counts only when dpkg's file list of
// the package names this very binary: a tar.gz copy in ~/.local/bin on a
// machine that also has the .deb installed still updates itself.
func detectLinuxPackage(exe, dpkgInfoDir string) string {
	lists, _ := filepath.Glob(filepath.Join(dpkgInfoDir, linuxPackageName+"*.list"))
	for _, l := range lists {
		base := strings.TrimSuffix(filepath.Base(l), ".list")
		if base != linuxPackageName && !strings.HasPrefix(base, linuxPackageName+":") {
			continue
		}
		if dpkgListHas(l, exe) {
			return linuxPkgDeb
		}
	}
	return linuxPkgNone
}

func dpkgListHas(list, path string) bool {
	f, err := os.Open(list)
	if err != nil {
		return false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if strings.TrimSpace(sc.Text()) == path {
			return true
		}
	}
	return false
}

// Query ownership of this exact executable, rather than detecting the distro:
// an archive copy beside an RPM install must still update as an archive.
func detectRPMPackage(exe string, owner func(string) (string, error)) string {
	name, err := owner(exe)
	if err == nil && strings.TrimSpace(name) == linuxPackageName {
		return linuxPkgRPM
	}
	return linuxPkgNone
}

func rpmOwner(exe string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "rpm", "-qf", "--queryformat", "%{NAME}", "--", exe).Output()
	return string(out), err
}
