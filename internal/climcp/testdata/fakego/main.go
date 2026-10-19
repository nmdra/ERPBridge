// fakego records the build process home without running a shell or compiling CLI.
package main

import "os"

func main() {
	home, err := os.UserHomeDir()
	if err == nil {
		err = os.WriteFile(os.Getenv("CLIMCP_BUILD_HOME_RECORD"), []byte(home), 0600)
	}
	if err != nil {
		_, _ = os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
}
