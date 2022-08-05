package main

import (
	"fmt"
	"os"

	"github.com/metal-toolbox/alloy/cmd"
)

func main() {
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
	}
}
