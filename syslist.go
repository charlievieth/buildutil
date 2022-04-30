package buildutil

// Valid GOOS/GOARCH pairings: `go tool dist list`
//
// aix/ppc64
// android/386
// android/amd64
// android/arm
// android/arm64
// darwin/amd64
// darwin/arm64
// dragonfly/amd64
// freebsd/386
// freebsd/amd64
// freebsd/arm
// freebsd/arm64
// illumos/amd64
// ios/amd64
// ios/arm64
// js/wasm
// linux/386
// linux/amd64
// linux/arm
// linux/arm64
// linux/mips
// linux/mips64
// linux/mips64le
// linux/mipsle
// linux/ppc64
// linux/ppc64le
// linux/riscv64
// linux/s390x
// netbsd/386
// netbsd/amd64
// netbsd/arm
// netbsd/arm64
// openbsd/386
// openbsd/amd64
// openbsd/arm
// openbsd/arm64
// openbsd/mips64
// plan9/386
// plan9/amd64
// plan9/arm
// solaris/amd64
// windows/386
// windows/amd64
// windows/arm
// windows/arm64

/*
type GoPlatform struct {
	GOOS         string
	GOARCH       string
	CgoSupported bool
	FirstClass   bool
}

func LoadGoPlatforms() ([]GoPlatform, error) {
	data, err := exec.Command("go", "tool", "dist", "list", "-json").Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			stderr := strings.TrimSpace(string(ee.Stderr))
			if i := strings.IndexByte(stderr, '\n'); i != -1 {
				stderr = stderr[:i]
			}
			return nil, fmt.Errorf("command `go tool dist list` failed: %s: %s",
				err, stderr)
		}
		return nil, fmt.Errorf("command `go tool dist list` failed: %s", err)
	}
	var ps []GoPlatform
	if err := json.Unmarshal(data, &ps); err != nil {
		return nil, err
	}
	return ps, err
}
*/

const goosList = "aix android darwin dragonfly freebsd hurd illumos ios js linux nacl netbsd openbsd plan9 solaris windows zos "
const goarchList = "386 amd64 amd64p32 arm armbe arm64 arm64be ppc64 ppc64le loong64 mips mipsle mips64 mips64le mips64p32 mips64p32le ppc riscv riscv64 s390 s390x sparc sparc64 wasm "
