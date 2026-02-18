package main

import "context"

// Test runs the tapes unit tests via "go test"
//
// +check
func (m *Masterblaster) Test(ctx context.Context) (string, error) {
	return m.goContainer().
		WithExec([]string{"go", "install", "github.com/onsi/ginkgo/v2/ginkgo"}).
		WithExec([]string{"ginkgo", "-p", "./..."}).
		Stdout(ctx)
}
