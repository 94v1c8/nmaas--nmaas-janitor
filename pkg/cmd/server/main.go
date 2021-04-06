package main

import (
	"fmt"
	"os"

	"bitbucket.software.geant.org/projects/NMAAS/repos/nmaas-janitor/pkg/api/cmd"
)

func main() {
	if err := cmd.RunServer(); err != nil {
		_, err = fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}
