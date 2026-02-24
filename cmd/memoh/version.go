package main

import (
	"fmt"

	"github.com/memohai/memoh/internal/version"
)

func runVersion() error {
	fmt.Printf("memoh %s\n", version.GetInfo())
	return nil
}
