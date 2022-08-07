//go:build go1.19

package buildutil

// The "unix" build constraint was added with g1.19
// https://tip.golang.org/doc/go1.19#go-unix
const matchUnixAndBoringCrypto = true
