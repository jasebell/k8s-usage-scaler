package main

import "github.com/jasebell/podmon/cmd"

func main() {
	cmd.Run("app=nginx", "default")
}
