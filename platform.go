package buildutil

import (
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

//go:generate go run -tags gen_platform_list genplatforms.go

// A GoPlatform is a supported GOOS/GOARCH for go and is generated via:
// `go tool dist list`
type GoPlatform struct {
	GOOS         string `json:"GOOS"`
	GOARCH       string `json:"GOARCH"`
	CgoSupported bool   `json:"CgoSupported"`
	FirstClass   bool   `json:"FirstClass"`
}

// LoadGoPlatforms loads the supported platforms supported by the
// go executable found on the PATH.
func LoadGoPlatforms() ([]GoPlatform, error) {
	data, err := exec.Command("go", "tool", "dist", "list", "-json").Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			stderr := strings.TrimSpace(string(ee.Stderr))
			if i := strings.IndexByte(stderr, '\n'); i != -1 {
				stderr = stderr[:i]
			}
			return nil, fmt.Errorf("buildutil: command `go tool dist list` failed: %s: %s",
				err, stderr)
		}
		return nil, fmt.Errorf("buildutil: command `go tool dist list` failed: %s", err)
	}
	var ps []GoPlatform
	if err := json.Unmarshal(data, &ps); err != nil {
		return nil, fmt.Errorf("buildutil: error unmarshalling GoPlatforms: %w", err)
	}
	return ps, err
}
